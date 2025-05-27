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
from typing import TextIO

import numpy as np
from stable_audio_tools import get_pretrained_model
from pyparsing import *

# Reduce fragmentation in PyTorch allocator
os.environ.setdefault("PYTORCH_CUDA_ALLOC_CONF", "expandable_segments:True")

import argparse
import json

from enum import Enum
from pathlib import Path
from sys import stdin
from tomllib import loads
from functools import partial

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

from pyparsing import (
    Literal,
    Word,
    Group,
    Forward,
    alphas,
    alphanums,
    Regex,
    Combine,
    OneOrMore,
    ParseException,
    CaselessKeyword,
    Suppress,
    delimitedList,
    Char,
    printables,
)
import math
import operator
from parser import *

STABLE_AUDIO_OPEN_1_0_PATH = "models/stable-audio-open-1.0"
STABLE_AUDIO_OPEN_SMALL_PATH = "models/stable-audio-open-small"

# Omit prompt, negprompt, and cfg_scale as these are no longer generic
default_cfg = {
    'prompt': None,
    'negative_prompt': None,
    'output': 'output.wav',
    'length': 30,
    'steps': 100,
    'cfg_scale': 7.0,
    'sampler': 'dpmpp-3m-sde',
    'progress_file': None,
    'init_audio': None,
    'seed': -1,
    'small': False,
}

class InvocationType(Enum):
    AUDIO2AUDIO = 1 # Unused

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

def infer(args, device, model, model_config, conditioning_tensors, negative_conditioning_tensors, audio):
    target_sample_rate = int(model_config.get("sample_rate"))
    sample_size = int(model_config.get("sample_size"))

    seed = args['seed']
    if seed < 0:
        # The geniuses at stable-audio didn't realize that 2^32-1 is out of bounds for a signed integer
        seed = np.random.randint(0, 2**31 - 1)
    print(f"Using seed: {seed}")

    with torch.no_grad():
        output = generate_diffusion_cond(
            model,
            steps=args['steps'],
            cfg_scale=args['cfg_scale'],
            conditioning_tensors=conditioning_tensors,
            negative_conditioning_tensors=negative_conditioning_tensors,
            init_audio=audio,
            init_noise_level=args.get("init_noise_level", 1.0),
            sample_size=sample_size,
            sample_rate=target_sample_rate,
            sampler_type=args['sampler'],
            device=device,
            sigma_min=0.3,
            sigma_max=500,
            seed=seed,
        )
    return output

def create_model(args):
    project_dir = get_project_dir()

    if args['small']:
        config_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_SMALL_PATH) / "model.ckpt"
        args['sampler'] = "pingpong" # sao-small only supports pingpong sampler

    else:
        config_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model_config.json"
        ckpt_path = (project_dir / STABLE_AUDIO_OPEN_1_0_PATH) / "model.ckpt"

    # If a progress file was indicated, create it to track progress, then delete it on cleanup

    if args['progress_file'] is not None:
        try:
            open(args['progress_file'], 'w').close()
        except Exception:
            pass

        def _cleanup():
            try:
                os.remove(args['progress_file'])
            except OSError:
                pass

        atexit.register(_cleanup)
        sys.stderr = ProgressWriter(sys.stderr, args['progress_file'])

    device = torch.device("cuda") if torch.cuda.is_available() else torch.device("cpu")


    with open(config_path) as f:
        model_config = json.load(f)

    if args['length'] != default_cfg['length']:
        model_config["sample_size"] = model_config["sample_rate"] * args['length']


    print(f"Creating model from config {config_path} on device {device}:")
    model = create_model_from_config(model_config)

    print(f" - loading weights from {ckpt_path}", flush=True)
    copy_state_dict(model, load_ckpt_state_dict(str(ckpt_path)))

    # Move model to device, set precision, and disable gradients
    model = model.to(device)
    if device.type == "cuda":
        model = model.half()
    model.eval()
    for p in model.parameters():
        p.requires_grad = False

    # Warm up GPU allocator
    if device.type == "cuda": torch.cuda.empty_cache()

    return device, model, model_config


def massage_audio(args, device, model_config):
    target_sample_rate = int(model_config.get("sample_rate"))

    n_samples = args["length"] * target_sample_rate

    audio2audio_conditioning = None

    if args['init_audio'] is not None:
        in_waveform, in_sample_rate = torchaudio.load(args['init_audio'])
        in_waveform = in_waveform[..., : in_sample_rate * args['length']]
        if in_sample_rate != target_sample_rate:
            print(f"Resampling input audio from sample rate {in_sample_rate} to {target_sample_rate}")
            resampler = torchaudio.transforms.Resample(orig_freq=in_sample_rate, new_freq=target_sample_rate)

            in_waveform = resampler(in_waveform)
            in_sample_rate = target_sample_rate
            print(" - done")
        in_waveform = in_waveform.to(device)
        if device.type == "cuda":
            print("Converting input audio to 16-bit")
            in_waveform = in_waveform.half()
            print(" - done")
        audio2audio_conditioning = (in_sample_rate, in_waveform)
    return audio2audio_conditioning


def unstack_tensor(device, model, length, t):
    prompt = t.pop()
    conditioning = [{
        "prompt": prompt,
        "seconds_start": 0,
        "seconds_total": length,
    }]
    conditioning_tensors = model.conditioner(conditioning, device)["prompt"][0]
    t.insert(0, conditioning_tensors)


def standard_tensors(args, device, model):
    negative_conditioning_tensors = None

    def condition(prompt, length, model, device):
        return model.conditioner([{"prompt": prompt, "seconds_start": 0, "seconds_total": length}], device)

    conditioning_tensors = condition(args["prompt"], args["length"], model, device)
    if args['negative_prompt'] is not None:
        negative_conditioning_tensors = condition(args["negative_prompt"], args["length"], model, device)

    return conditioning_tensors, negative_conditioning_tensors


def sum_tensors(args, device, model):
    negative_conditioning_tensors = None

    def sum_over_prompts(prompts, length, device, model):
        # Target starts its life as unconditioned generation. This is, for some reason, fine.
        target = model.conditioner(
            [{"prompt": "", "seconds_start": 0, "seconds_total": length}],
            device)
        sum = torch.zeros_like(target['prompt'][0])

        for elem in prompts:
            prompt_text = elem['prompt']
            weight = elem['weight']
            if weight == 0:
                continue

            sum += (weight * model.conditioner([{"prompt": prompt_text, "seconds_start": 0, "seconds_total": args['length']}], device)['prompt'][0])

        # continuous_transformer does not ***CURRENTLY*** support attention masks, so we use the unconditional diffusion mask.
        # If this shit breaks after updating the model, now you know why.
        target['prompt'] = (sum, torch.ones_like(target['prompt'][1]))
        return target

    conditioning_tensors = sum_over_prompts(args['prompts'], args['length'], device, model)

    if args.get('negative_prompts', None) is not None:
        negative_conditioning_tensors = sum_over_prompts(args['negative_prompts'], args['length'], device, model)

    if args.get('normalize_embeddings', False):
        conditioning_tensors = np.divide(conditioning_tensors, len(args['prompts']))
        if negative_conditioning_tensors is not None:
            negative_conditioning_tensors = np.divide(negative_conditioning_tensors, len(args['negative_prompts']))

    return conditioning_tensors, negative_conditioning_tensors


def freaky_tensors(args, device, model):
    eval_prompt = partial(unstack_tensor, device, model, args['length'])

    def expr2tensor(prompt):
        exprStack[:] = []
        try:
            results = BNF(eval_prompt).parseString(prompt, parseAll=True)
            val = evaluate_stack(exprStack[:])
        except ParseException as pe:
            print(prompt, "failed parse:", str(pe))
            exit(1)
        return val

    negative_conditioning_tensors = None

    uncond_spec = [{"prompt": "", "seconds_start": 0, "seconds_total": args['length']}]
    uncond_tensors = model.conditioner(uncond_spec, device)
    uncond_tensors['prompt'] = (expr2tensor(args['eprompt']), uncond_tensors['prompt'][1])
    conditioning_tensors = uncond_tensors

    if args.get('enegative_prompt', None) is not None:
        uncond_negative_tensors = model.conditioner(uncond_spec, device)
        uncond_negative_tensors['prompt'] = (expr2tensor(args['eprompt']), uncond_tensors['prompt'][1])
        negative_conditioning_tensors = uncond_negative_tensors

    return conditioning_tensors, negative_conditioning_tensors

def toml_prompt():
    input = stdin.read()
    toml = loads(input)
    args = default_cfg
    if toml['config'] is not None:
        args = default_cfg | toml['config']

    prompts = toml.get('prompts', None)
    if prompts is None:
        return {}

    if prompts.get('eprompt', None) is not None:
        args['eprompt'] = prompts['eprompt']
    else:
        prompts = [{'prompt': k, 'weight': v} for k, v in prompts.items()]
        args['prompts'] = prompts

    negative_prompts = toml.get('negative_prompts', None)
    if negative_prompts is not None:
        if negative_prompts.get('eprompt', None) is not None:
            args['enegative_prompt'] = negative_prompts['eprompt']
        else:
            negative_prompts = [{'prompt': k, 'weight': v} for k, v in negative_prompts.items()]
            args['negative_prompts'] = negative_prompts

    args['inpaint'] = toml.get('inpaint', None)
    return args

def main() -> None:
    parser = argparse.ArgumentParser(
        description="Generate audio with Stable Audio Open 1.0"
    )
    # Move defaults into the default dict
    parser.add_argument("--prompt", help="Text prompt for audio generation")
    parser.add_argument("--negative_prompt", help="Negative prompt for audio generation")
    parser.add_argument("--output", help="Output WAV file path")
    parser.add_argument("--length", type=float, help="Length in seconds")
    parser.add_argument("--steps", type=int, help="Number of diffusion steps")
    parser.add_argument("--cfg_scale", type=float, default=7.0, help="CFG scale")
    parser.add_argument("--sampler", help="Sampler type")
    parser.add_argument("--progress_file", help="File to write progress output to")
    parser.add_argument("--init_audio", help="Path to a WAV file to condition on (audio2audio)")
    parser.add_argument("--seed", type=int, help="Integer seed used for randomness in audio generation")
    parser.add_argument("--small", action="store_true", help="If set, uses the small version of Stable Audio Open")
    parser.add_argument("--eprompt", type=str, help="Prompt expression")
    parser.add_argument("--enegative_prompt", type=str, help="Negative prompt expression")
    parser.add_argument("--toml", action="store_true", help="Provide config as a TOML file to stdin")
    args = parser.parse_args().__dict__
    args = {**default_cfg, **{k: v for k, v1 in args.items() if (v := v1) is not None}} # Overwrite default vals when specified
    args['cfg_scale'] = 7.0 if args['init_audio'] is None and args['cfg_scale'] == 7.0 else 150.0
    toml = args.get('toml', False)
    if toml:
        args = toml_prompt()

    if args.get('prompt', None) is None and args.get('prompts', None) is None and args.get('eprompt', None) is None:
        raise ValueError("Promptism :^)")

    device, model, model_config = create_model(args)

    audio_cond = None
    if args.get('init_audio', False):
        audio_cond = massage_audio(args, device, model_config)

    pe, npe = None, None
    if args.get('prompt', None) is not None:
        pe, npe = standard_tensors(args, device, model)
    elif args.get('eprompt', None) is not None:
        pe, npe = freaky_tensors(args, device, model)
    elif args.get('prompts', None) is not None:
        pe, npe = sum_tensors(args, device, model)

    output = infer(args, device, model, model_config, pe, npe, audio_cond)


    # Reshape and normalize to PCM16
    audio = rearrange(output, "b d n -> d (b n)")
    audio = audio.to(torch.float32)
    audio = audio.div(torch.max(torch.abs(audio))).clamp(-1, 1) #???
    audio = audio.mul(32767).to(torch.int16).cpu()

    # Save output
    torchaudio.save(args['output'], audio, model_config.get("sample_rate"))
    print(f"Saved audio to {args['output']}", flush=True)


if __name__ == "__main__":
    main()