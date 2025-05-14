import argparse
import os
import subprocess
import sys
from pathlib import Path

MODELS_DIR_NAME = "models"

def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Clone a Hugging Face repo locally")
    parser.add_argument(
        "repo_id", help="Hugging face namespace/repo; eg. stabilityai/stable-audio-open-1.0"
    )
    return parser.parse_args()

def main() -> None:
    args = parse_args()

    try:
        hf_user_id = os.environ["HF_USER_ID"]
        hf_token = os.environ["HF_TOKEN"]
    except KeyError:
        sys.exit(1, "You need to set HF_USER_ID and HF_TOKEN before running this script")

    repo = args.repo_id
    print(f"Cloning Hugging Face repo '{repo}' as user '{hf_user_id}'")

    models_dir = Path(__file__).parent.parent / MODELS_DIR_NAME
    if not models_dir.is_dir():
        print(f"Models dir '{models_dir}' does not exist; creating...")
        os.mkdir(str(models_dir))
    os.chdir(models_dir)

    assert(Path(os.getcwd()).name == MODELS_DIR_NAME)

    url = f"https://{hf_user_id}:{hf_token}@huggingface.co/{repo}"
    subprocess.run(["git", "clone", url])


if __name__ == "__main__":
    main()