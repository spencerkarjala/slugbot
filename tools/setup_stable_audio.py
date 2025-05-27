#!/usr/bin/env python3
"""
setup_stable_audio.py

Creates a local Conda environment in `./.conda-env`, installs Python 3.11 and pip,
then installs stable-audio-tools and its dependencies without affecting your global system.

Usage:
    python setup_stable_audio.py

Requirements:
- `conda` or `mamba` executable on your PATH
"""
import sys
import shutil
import subprocess
from pathlib import Path

# Configuration
ENV_DIR = Path(".conda-env")
PYTHON_VERSION = "3.11"


def run(cmd):
    print(">>", " ".join(cmd))
    subprocess.run(cmd, check=True)


def main():
    # Locate conda or mamba
    conda_cmd = shutil.which("mamba") or shutil.which("conda")
    if not conda_cmd:
        sys.exit("Error: neither 'mamba' nor 'conda' found on PATH. Please install one of these.")

    # Create environment if missing
    if not ENV_DIR.exists():
        print(f"Creating Conda environment at {ENV_DIR} with Python {PYTHON_VERSION}...")
        run(
            [
                conda_cmd,
                "create",
                "--yes",
                "--prefix",
                str(ENV_DIR),
                f"python={PYTHON_VERSION}",
                "pip",
            ]
        )
    else:
        print(f"Using existing Conda environment at {ENV_DIR}")

    # Install or upgrade packages via pip
    pip_cmd = [conda_cmd, "run", "--prefix", str(ENV_DIR), "pip", "install", "--upgrade"]
    print("Upgrading pip, setuptools, wheel, and numpy...")
    run(pip_cmd + ["pip", "setuptools", "wheel", "numpy"])

    print("Installing stable-audio-tools...")
    run(pip_cmd + ["--prefer-binary", "stable-audio-tools"])

    print("Verifying installation...")
    run([conda_cmd, "run", "--prefix", str(ENV_DIR), "pip", "show", "stable-audio-tools"])

    print("Done.")


if __name__ == "__main__":
    main()
