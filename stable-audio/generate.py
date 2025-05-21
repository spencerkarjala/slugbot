#!/usr/bin/env python
"""
Generate audio using Stability AI's Stable Audio Open 1.0 model.

Usage (after creating a Conda env at ./.conda-env):
  conda run --prefix ./.conda-env python stable-audio/generate.py \
      --prompt "128 BPM tech house drum loop" --output output.wav
"""
import atexit
import os
import shutil
import sys
import tempfile
from functools import reduce
import tomllib
from typing import TextIO

import numpy as np
from stable_audio_tools import get_pretrained_model

# Reduce fragmentation in PyTorch allocator
os.environ.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")

import argparse
import json

from enum import Enum
from pathlib import Path
from sys import stdin
from tomllib import loads

import torch
import torchaudio
from einops import rearrange
from stable_audio_tools.models.factory import create_model_from_config
from stable_audio_tools.models.utils import load_ckpt_state_dict
from stable_audio_tools.models.utils import copy_state_dict
from stable_audio_tools.inference.generation import (
    generate_diffusion_cond,
    generate_diffusion_cond_inpaint,
)
from stable_audio_tools.inference.utils import prepare_audio

STABLE_AUDIO_OPEN_1_0_PATH = "models/stable-audio-open-1.0"
STABLE_AUDIO_OPEN_SMALL_PATH = "models/stable-audio-open-small"

# Omit prompt, negprompt, and cfg_scale as these are no longer generic
default_cfg = {
    "prompt": None,
    "negative_prompt": None,
    "output": "output.wav",
    "length": 30,
    "steps": 100,
    "cfg_scale": 7.0,
    "sampler": "dpmpp-3m-sde",
    "progress_file": None,
    "init_audio": None,
    "seed": -1,
    "small": False,
}


class InvocationType(Enum):
    AUDIO2AUDIO = 1  # Unused
    SPROMPT = 2
    NPROMPT = 3
    INPAINT = 4


class ProgressWriter:
    """
    Wraps a stream to capture tqdm-style progress bars (which use '\r')
    and write the latest line to a file on each carriage return.
    """

    def __init__(self, stream: TextIO, fname: str) -> None:
        self._stream = stream
        self._fname = fname

    def write(self, data: str) -> None:
        # Forward incoming data back to the original stream
        self._stream.write(data)

        # If it doesn't start with a carriage return, it's probably not a progress bar,
        # so just skip it
        if not data or data[0] != "\r":
            return

        try:
            with open(self._fname, "w") as f:
                f.write("`" + data[1:].rstrip("\n") + "`")
        except Exception:
            pass

    def flush(self) -> None:
        self._stream.flush()


def get_project_dir(start_dir: Path = Path.cwd()) -> Path:
    """Walk upward until a .git directory is found"""
    for p in (start_dir, *start_dir.parents):
        if (p / ".git").is_dir():
            return p
    raise FileNotFoundError(f"No project dir found as a parent of '{start_dir}'")


def trim_audio_inplace(filepath, seconds):
    audio, sample_rate = torchaudio.load(filepath)
    num_samples = int(seconds * sample_rate)
    trimmed_audio = audio[:, :num_samples]

    tmp_out = filepath + ".trimmed.tmp.wav"
    torchaudio.save(tmp_out, trimmed_audio, sample_rate)
    os.replace(tmp_out, filepath)


def infer(
    args,
    audio,
    conditioning_tensors,
    device,
    model,
    negative_conditioning_tensors,
    sample_size,
    seed,
    target_sample_rate,
):
    with torch.no_grad():
        output = generate_diffusion_cond(
            model,
            steps=args["steps"],
            cfg_scale=args["cfg_scale"],
            conditioning_tensors=conditioning_tensors,
            negative_conditioning_tensors=negative_conditioning_tensors,
            init_audio=audio,
            sample_size=sample_size,
            sample_rate=target_sample_rate,
            sampler_type=args["sampler"],
            device=device,
            sigma_min=0.3,
            sigma_max=500,
            seed=seed,
        )
    return output


def infer_inpaint(
    args,
    audio,
    mask,
    conditioning_tensors,
    device,
    model,
    negative_conditioning_tensors,
    sample_size,
    seed,
    target_sample_rate,
):
    with torch.no_grad():
        output = generate_diffusion_cond_inpaint(
            model,
            steps=args["steps"],
            cfg_scale=args["cfg_scale"],
            conditioning_tensors=conditioning_tensors,
            negative_conditioning_tensors=negative_conditioning_tensors,
            init_audio=audio,
            inpaint_audio=audio,
            inpaint_mask=mask,
            sample_size=sample_size,
            sampler_type=args["sampler"],
            device=device,
            sigma_min=0.3,
            sigma_max=500,
            seed=seed,
        )

    return output


def shared_model_invocation(args, inv_type) -> None:
    project_dir = get_project_dir()

    # Switch between models
    if args["small"]:
        config_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model.ckpt"
        # manually override sampler, since SAO Small only supports pingpong sampler
        args["sampler"] = "pingpong"
        args["cfg_scale"] = args.get("cfg_scale", 6.0)
    else:
        config_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model.ckpt"

    # If a progress file was indicated, create it to track progress, then delete it on cleanup
    if args["progress_file"] is not None:
        try:
            open(args["progress_file"], "w").close()
        except Exception:
            pass

        def _cleanup():
            try:
                os.remove(args["progress_file"])
            except OSError:
                pass

        atexit.register(_cleanup)
        sys.stderr = ProgressWriter(sys.stderr, args["progress_file"])

    # Select device
    device = torch.device("cuda") if torch.cuda.is_available() else torch.device("cpu")
    print(f"Using device: {device}", flush=True)

    # Parse the seed if it's present
    seed = args["seed"]
    if seed < -1:
        raise ValueError("Seed must be >= -1")
    if seed == -1:
        # The geniuses at stable-audio didn't realize that 2^32-1 is out of bounds for a signed integer
        seed = np.random.randint(0, 2**31 - 1)
    print(f"Using seed: {seed}")

    # Load model configuration
    print(f"Loading model config from {config_path}", flush=True)
    with open(config_path) as f:
        model_config = json.load(f)

    if args["length"] != default_cfg["length"]:
        model_config["sample_size"] = model_config["sample_rate"] * args["length"]

    # Instantiate model
    print("Creating model from config...", flush=True)
    print(f"Model config's sample_size is {model_config['sample_size']}")
    model = create_model_from_config(model_config)

    # Load weights from local checkpoint
    print(f"Loading checkpoint from {ckpt_path}", flush=True)
    state_dict = load_ckpt_state_dict(str(ckpt_path))
    copy_state_dict(model, state_dict)

    # Move model to device, set precision, and disable gradients
    model = model.to(device)
    if device.type == "cuda":
        model = model.half()
    model.eval()
    for p in model.parameters():
        p.requires_grad = False

    target_sample_rate = int(model_config.get("sample_rate"))
    sample_size = int(model_config.get("sample_size"))

    n_samples = args["length"] * target_sample_rate

    audio2audio_conditioning = None
    if args["init_audio"] is not None:
        print(f"Using input audio file '{args['init_audio']}'")
        in_waveform, in_sample_rate = torchaudio.load(args["init_audio"])
        in_waveform = in_waveform[..., : int(in_sample_rate * args["length"])]
        if in_sample_rate != target_sample_rate:
            print(
                f"Resampling input audio from sample rate {in_sample_rate} to {target_sample_rate}..."
            )
            resampler = torchaudio.transforms.Resample(
                orig_freq=in_sample_rate, new_freq=target_sample_rate
            )
            in_waveform = resampler(in_waveform)
            in_sample_rate = target_sample_rate
            print("...done resampling")
        in_waveform = in_waveform.to(device)
        if device.type == "cuda":
            print("Converting input audio to 16-bit...")
            in_waveform = in_waveform.half()
            print("...done converting")
        audio2audio_conditioning = (in_sample_rate, in_waveform)

    # Warm up GPU allocator
    if device.type == "cuda":
        torch.cuda.empty_cache()

    conditioning_tensors = None
    negative_conditioning_tensors = None

    output = None
    match inv_type:
        case InvocationType.SPROMPT:
            conditioning = [
                {
                    "prompt": args["prompt"],
                    "seconds_start": 0,
                    "seconds_total": args["length"],
                }
            ]
            conditioning_tensors = model.conditioner(conditioning, device)

            negative_conditioning = None
            if args["negative_prompt"] is not None:
                negative_conditioning = [
                    {
                        "prompt": args["negative_prompt"],
                        "seconds_start": 0,
                        "seconds_total": args["length"],
                    }
                ]
                negative_conditioning_tensors = model.conditioner(negative_conditioning, device)

            print(f"Generating {args['length']}s audio with {args['steps']} steps and cfg_scale={args['cfg_scale']}...", flush=True)
            output = infer(
                args,
                audio2audio_conditioning,
                conditioning_tensors,
                device,
                model,
                negative_conditioning_tensors,
                sample_size,
                seed,
                target_sample_rate,
            )

        case InvocationType.NPROMPT | InvocationType.INPAINT:
            uncond_spec = [{"prompt": "", "seconds_start": 0, "seconds_total": args["length"]}]
            uncond_tensors_batched = model.conditioner(uncond_spec, device)
            uncond_negative_tensors_batched = model.conditioner(uncond_spec, device)

            # Initialize a zero tensor with the correct shape and device for the sum
            prompt_embedding_template = uncond_tensors_batched["prompt"][0]
            prompt_embedding_template = torch.zeros_like(prompt_embedding_template)

            # Iterate through prompts and their weights
            for elem in args["prompts"]:
                prompt_text = elem["prompt"]
                weight = elem["weight"]
                if weight == 0:
                    continue

                current_prompt_spec = [
                    {"prompt": prompt_text, "seconds_start": 0, "seconds_total": args["length"]}
                ]
                current_cond_tensor = model.conditioner(current_prompt_spec, device)["prompt"][0]

                prompt_embedding_template += weight * current_cond_tensor

            uncond_tensors_batched["prompt"] = (
                prompt_embedding_template,
                uncond_tensors_batched["prompt"][1],
            )
            conditioning_tensors = uncond_tensors_batched

            if args.get("neg_prompts") is not None:
                negative_prompt_embedding_template = uncond_tensors_batched["prompt"][0]
                negative_prompt_embedding_template = torch.zeros_like(
                    negative_prompt_embedding_template
                )

                for elem in args["neg_prompts"]:
                    prompt_text = elem["prompt"]
                    weight = elem["weight"]
                    if weight == 0:
                        continue

                    current_prompt_spec = [
                        {"prompt": prompt_text, "seconds_start": 0, "seconds_total": args["length"]}
                    ]
                    current_cond_tensor = model.conditioner(current_prompt_spec, device)["prompt"][
                        0
                    ]

                    negative_prompt_embedding_template += weight * current_cond_tensor

                uncond_negative_tensors_batched["prompt"] = (
                    negative_prompt_embedding_template,
                    uncond_negative_tensors_batched["prompt"][1],
                )
                negative_conditioning_tensors = uncond_negative_tensors_batched

            if inv_type == InvocationType.INPAINT:
                print(f"Inpainting {args['init_audio']} with {args['steps']} steps and cfg_scale={args['cfg_scale']}...", flush=True)

                inpaint_mask = torch.ones(1, sample_size, device=device)
                for _slice_name, time_range_seconds in args["inpaint"].items():
                    start_sec, end_sec = time_range_seconds

                    start_sample = int(start_sec * target_sample_rate)
                    end_sample = int(end_sec * target_sample_rate)

                    # Clamp to audio bounds and ensure valid range
                    start_sample = max(0, start_sample)
                    end_sample = min(n_samples, end_sample)

                    if start_sample < end_sample:
                        inpaint_mask[start_sample:end_sample] = 0

                output = infer_inpaint(
                    args,
                    audio2audio_conditioning,
                    inpaint_mask,
                    conditioning_tensors,
                    device,
                    model,
                    negative_conditioning_tensors,
                    sample_size,
                    seed,
                    target_sample_rate,
                )

            else:
                print(
                    f"Generating {args['length']}s audio with {args['steps']} steps and cfg_scale={args['cfg_scale']}...", flush=True
                )
                output = infer(
                    args,
                    audio2audio_conditioning,
                    conditioning_tensors,
                    device,
                    model,
                    negative_conditioning_tensors,
                    sample_size,
                    seed,
                    target_sample_rate,
                )

    if device.type == "cuda":
        torch.cuda.empty_cache()

    # Reshape and normalize to PCM16
    audio = rearrange(output, "b d n -> d (b n)")
    audio = audio.to(torch.float32)
    audio = audio.div(torch.max(torch.abs(audio))).clamp(-1, 1)
    audio = audio.mul(32767).to(torch.int16).cpu()

    # Save output
    torchaudio.save(args["output"], audio, target_sample_rate)
    print(f"Saved audio to {args['output']}", flush=True)

    trim_audio_inplace(args["output"], args["length"])


def simple_prompt() -> None:
    # Some logic here describes how the args struct is created;
    # Either we had a .saudio invocation or a ```toml invocation
    parser = argparse.ArgumentParser(description="Generate audio with Stable Audio Open 1.0")
    # Move defaults into the default dict
    parser.add_argument("--prompt", required=True, help="Text prompt for audio generation")
    parser.add_argument("--negative_prompt", help="Negative prompt for audio generation")
    parser.add_argument("--output", help="Output WAV file path")
    parser.add_argument("--length", type=float, help="Length in seconds")
    parser.add_argument("--steps", type=int, help="Number of diffusion steps")
    parser.add_argument("--cfg_scale", type=float, default=7.0, help="CFG scale")
    parser.add_argument("--sampler", help="Sampler type")
    parser.add_argument("--progress_file", help="File to write progress output to")
    parser.add_argument("--init_audio", help="Path to a WAV file to condition on (audio2audio)")
    parser.add_argument(
        "--seed", type=int, help="Integer seed used for randomness in audio generation"
    )
    parser.add_argument(
        "--small", action="store_true", help="If set, uses the small version of Stable Audio Open"
    )
    args = parser.parse_args().__dict__
    args = {
        **default_cfg,
        **{k: v for k, v1 in args.items() if (v := v1) is not None},
    }  # Overwrite default vals when specified

    if args["init_audio"] is not None and args["cfg_scale"] == parser.get_default("cfg_scale"):
        args["cfg_scale"] = 125.0

    # args["cfg_scale"] = args["strength"]
    # args["cfg_scale"] = args["strength"] if args["init_audio"] is None else 150.0

    shared_model_invocation(args, InvocationType.SPROMPT)


def toml_prompt() -> None:
    parser = argparse.ArgumentParser(description="Generate audio with Stable Audio Open 1.0")
    parser.add_argument("--output", type=str, default="", help="Output WAV file path")
    parser.add_argument("--progress_file", type=str, default="", help="File to write progress output to")
    parser.add_argument("--init_audio", type=str, default="", help="Path to a WAV file to condition on (audio2audio)")
    parser.add_argument("--toml", action="store_true", help="Read TOML from stdin")
    args_in = parser.parse_args().__dict__
    try:
        input = stdin.read()
        toml = loads(input)
        args = default_cfg
        if toml.get("config") is not None:
            args = default_cfg | toml["config"]

        print("got TOML: ")
        print(toml)
        prompts = toml.get("prompts", None)
        if prompts is None:
            print("No prompts received. Exiting...")
            exit(1)
        prompts = [{"prompt": k, "weight": v} for k, v in prompts.items()]
        args["prompts"] = prompts

        if args_in.get("output"):
            args["output"] = args_in.get("output")
        if args_in.get("progress_file"):
            args["progress_file"] = args_in.get("progress_file")
        if args_in.get("init_audio"):
            args["init_audio"] = args_in.get("init_audio")

        neg_prompts = toml.get("neg_prompts", None)
        if neg_prompts is not None:
            neg_prompts = [{"prompt": k, "weight": v} for k, v in neg_prompts.items()]
            args["neg_prompts"] = neg_prompts

        args["inpaint"] = toml.get("inpaint", None)


        shared_model_invocation(
            args, InvocationType.NPROMPT if args["inpaint"] is None else InvocationType.INPAINT
        )
    except tomllib.TOMLDecodeError as e:
        print(f"rain into TOML decode error: {e}")


def main() -> None:
    # if there's something on stdin, assume it's a TOML prompt
    if "--toml" in sys.argv:
        print("Using !!!TOML PROMPT!!! !!!EXPERIMENTAL!!!")
        toml_prompt()
    else:
        simple_prompt()


if __name__ == "__main__":
    main()
