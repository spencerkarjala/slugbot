#!/usr/bin/env python3

import shutil
import subprocess
import sys
from pathlib import Path

ENV_DIR = Path(".conda/general-dsp")
ENV_CONFIG = Path("py/env/general-dsp.yml")


def run(cmd):
    print(">>", " ".join(cmd))
    subprocess.run(cmd, check=True)


def main():
    conda_cmd = shutil.which("mamba") or shutil.which("conda")
    if not conda_cmd:
        sys.exit("Error: neither 'mamba' nor 'conda' found on PATH. Please install one of these.")

    if not ENV_CONFIG.exists():
        sys.exit(f"Error: Conda environment config '{ENV_CONFIG}' does not exist.")

    if not ENV_DIR.exists():
        print(f"Creating Conda environment at {ENV_DIR} from file {ENV_CONFIG}...")
        run(
            [
                conda_cmd,
                "env",
                "create",
                "--yes",
                "--prefix",
                str(ENV_DIR),
                "--file",
                str(ENV_CONFIG),
            ]
        )
    else:
        print(f"Updating existing conda environment at {ENV_DIR}.")
        run(
            [
                conda_cmd,
                "env",
                "update",
                "--yes",
                "--prefix",
                str(ENV_DIR),
                "--file",
                str(ENV_CONFIG),
                "--prune",
            ]
        )


if __name__ == "__main__":
    main()
