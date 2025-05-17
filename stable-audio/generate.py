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
from typing import TextIO

# Reduce fragmentation in PyTorch allocator
os.environ.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")

import argparse
import json
from pathlib import Path
import torch
import torchaudio
from einops import rearrange
from stable_audio_tools.models.factory import create_model_from_config
from stable_audio_tools.models.utils import load_ckpt_state_dict
from stable_audio_tools.models.utils import copy_state_dict
from stable_audio_tools.inference.generation import generate_diffusion_cond
from stable_audio_tools.inference.utils import prepare_audio

STABLE_AUDIO_OPEN_1_0_PATH = "models/stable-audio-open-1.0"
STABLE_AUDIO_OPEN_SMALL_PATH = "models/stable-audio-open-small"

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
        if not data or data[0] != '\r':
            return
        
        try:
            with open(self._fname, 'w') as f:
                f.write(data[1:].rstrip('\n'))
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

def main() -> None:
    parser = argparse.ArgumentParser(
        description="Generate audio with Stable Audio Open 1.0"
    )
    parser.add_argument("--prompt", required=True, help="Text prompt for audio generation")
    parser.add_argument("--negative_prompt", default="", help="Negative prompt for audio generation")
    parser.add_argument("--output", default="output.wav", help="Output WAV file path")
    parser.add_argument("--length", type=float, default=30.0, help="Length in seconds")
    parser.add_argument("--steps", type=int, default=100, help="Number of diffusion steps")
    parser.add_argument("--cfg_scale", type=float, default=7.0, help="CFG scale")
    parser.add_argument("--sampler", default="dpmpp-3m-sde", help="Sampler type")
    parser.add_argument("--progress_file", default="", help="File to write progress output to")
    parser.add_argument("--init_audio", default=None, help="Path to a WAV file to condition on (audio2audio)")
    parser.add_argument("--seed", default="", help="Integer seed used for randomness in audio generation")
    parser.add_argument("--small", action="store_true", help="If set, uses the small version of Stable Audio Open")
    args = parser.parse_args()

    # # Scale cfg_scale to its expected values; much higher for audio2audio prompts
    # if not args.init_audio:
    #     args.cfg_scale = args.cfg_scale * 7.0
    # else:
    #     args.cfg_scale = args.cfg_scale * 150.0

    project_dir = get_project_dir()

    # Switch between models
    if args.small:
        config_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model.ckpt"
        # manually override sampler, since SAO Small only supports pingpong sampler
        args.sampler = "pingpong"
        # args.length = 12
        args.cfg_scale = args.cfg_scale * 6.0 / 7.0
    else:
        config_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model.ckpt"

    # If a progress file was indicated, create it to track progress, then delete it on cleanup
    if args.progress_file:
        try:
            open(args.progress_file, 'w').close()
        except Exception:
            pass
        def _cleanup():
            try:
                os.remove(args.progress_file)
            except OSError:
                pass
        atexit.register(_cleanup)
        sys.stderr = ProgressWriter(sys.stderr, args.progress_file)

    # Select device
    device = torch.device("cuda") if torch.cuda.is_available() else torch.device("cpu")
    print(f"Using device: {device}", flush=True)

    # Parse the seed if it's present
    seed_value = -1
    if args.seed and args.seed != "-1":
        seed_value = int(args.seed)
        if seed_value < 0:
            raise ValueError("Seed needs to be a positive integer")
        print("Using seed: ", seed_value)

    # Load model configuration
    print(f"Loading model config from {config_path}", flush=True)
    with open(config_path) as f:
        model_config = json.load(f)

    if args.length != parser.get_default("length"):
        model_config["sample_size"] = model_config["sample_rate"] * args.length

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

    audio2audio_conditioning = None
    if args.init_audio:
        in_waveform, in_sample_rate = torchaudio.load(args.init_audio)
        if in_sample_rate != target_sample_rate:
            print(f"Resampling input audio from sample rate {in_sample_rate} to {target_sample_rate}...")
            resampler = torchaudio.transforms.Resample(orig_freq=in_sample_rate, new_freq=target_sample_rate)
            in_waveform = resampler(in_waveform)
            in_sample_rate = target_sample_rate
            print("...done resampling")
        in_waveform = in_waveform.to(device)
        if device.type == "cuda":
            print("Converting input audio to 16-bit...")
            in_waveform = in_waveform.half()
            print("...done converting")
        audio2audio_conditioning = (in_sample_rate, in_waveform)

    # Prepare conditioning
    conditioning = [{
        "prompt": args.prompt,
        "seconds_start": 0,
        "seconds_total": args.length,
    }]

    # Prepare negative conditioning
    negative_conditioning = None
    if args.negative_prompt:
        negative_conditioning = [{
            "prompt": args.negative_prompt,
            "seconds_start": 0,
            "seconds_total": args.length,
        }]

    # Warm up GPU allocator
    if device.type == "cuda": torch.cuda.empty_cache()

    # Run generation without tracking gradients
    print(f"Generating {args.length}s audio with {args.steps} steps...", flush=True)
    with torch.no_grad():
        output = generate_diffusion_cond(
            model,
            steps=args.steps,
            cfg_scale=args.cfg_scale,
            conditioning=conditioning,
            negative_conditioning=negative_conditioning,
            init_audio=audio2audio_conditioning,
            sample_size=sample_size,
            sample_rate=target_sample_rate,
            sampler_type=args.sampler,
            device=device,
            sigma_min=0.3,
            sigma_max=500,
            seed=seed_value,
        )

    # Free any unused GPU memory
    if device.type == "cuda": torch.cuda.empty_cache()

    # Reshape and normalize to PCM16
    audio = rearrange(output, "b d n -> d (b n)")
    audio = audio.to(torch.float32)
    audio = audio.div(torch.max(torch.abs(audio))).clamp(-1, 1)
    audio = audio.mul(32767).to(torch.int16).cpu()

    # Save output
    torchaudio.save(args.output, audio, target_sample_rate)
    print(f"Saved audio to {args.output}", flush=True)

    trim_audio_inplace(args.output, args.length)


if __name__ == "__main__":
    main()
