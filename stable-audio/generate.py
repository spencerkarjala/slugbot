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
from stable_audio_tools.training.utils import copy_state_dict
from stable_audio_tools.inference.generation import generate_diffusion_cond

MODEL_CONFIG_PATH = "models/stable-audio-open-1.0/model_config.json"
MODEL_CHECKPOINT_PATH = "models/stable-audio-open-1.0/model.ckpt"

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

    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmpfile:
        torchaudio.save(tmpfile.name, trimmed_audio, sample_rate)
        shutil.move(tmpfile.name, filepath)

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
    args = parser.parse_args()

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

    init_audio_tensor = None
    if args.init_audio:
        init_audio, _sr = torchaudio.load(args.init_audio)
        # if sample rates mismatch, optionally resample here
        init_audio_tensor = init_audio.to(device)

    project_dir = get_project_dir()

    # Load model configuration
    config_path = project_dir / MODEL_CONFIG_PATH
    print(f"Loading model config from {config_path}", flush=True)
    with open(config_path) as f:
        model_config = json.load(f)

    # Instantiate model
    print("Creating model from config...", flush=True)
    model = create_model_from_config(model_config)

    # Load weights from local checkpoint
    ckpt_path = project_dir / MODEL_CHECKPOINT_PATH
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

    sample_rate = model_config.get("sample_rate")
    sample_size = model_config.get("sample_size")

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

    # Prepare audio2audio conditioning
    audio2audio_conditioning = None
    if init_audio_tensor is not None:
        audio2audio_conditioning = init_audio_tensor

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
            sample_rate=sample_rate,
            sampler_type=args.sampler,
            device=device,
            sigma_min=0.3,
            sigma_max=500,
        )

    # Free any unused GPU memory
    if device.type == "cuda": torch.cuda.empty_cache()

    # Reshape and normalize to PCM16
    audio = rearrange(output, "b d n -> d (b n)")
    audio = audio.to(torch.float32)
    audio = audio.div(torch.max(torch.abs(audio))).clamp(-1, 1)
    audio = audio.mul(32767).to(torch.int16).cpu()

    # Save output
    torchaudio.save(args.output, audio, sample_rate)
    print(f"Saved audio to {args.output}", flush=True)

    trim_audio_inplace(args.output, args.length)


if __name__ == "__main__":
    main()
