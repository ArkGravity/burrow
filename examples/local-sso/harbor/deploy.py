"""Manage a local Harbor example using its official, pinned prepare image."""

import argparse
import json
from pathlib import Path
import re
import subprocess


HARBOR_VERSION = "v2.15.2"
EXAMPLE_DIR = Path(__file__).resolve().parent
RUNTIME_DIR = EXAMPLE_DIR / "runtime"
COMPOSE_PROJECT = "burrow-local-sso-harbor"


def run(args):
    subprocess.run(args, check=True)


def prepare():
    config_dir = RUNTIME_DIR / "common" / "config"
    data_dir = EXAMPLE_DIR / "data"
    log_dir = EXAMPLE_DIR / "logs"
    input_dir = RUNTIME_DIR / "input"
    for directory in (config_dir, data_dir, log_dir, input_dir):
        directory.mkdir(parents=True, exist_ok=True)
    config = (EXAMPLE_DIR / "harbor.yml").read_text(encoding="utf-8")
    config = config.replace("__DATA_DIR__", json.dumps(str(data_dir)))
    config = config.replace("__LOG_DIR__", json.dumps(str(log_dir)))
    (input_dir / "harbor.yml").write_text(config, encoding="utf-8")
    run(
        [
            "docker", "run", "--rm", "--platform", "linux/amd64",
            "--volume", f"{input_dir}:/input:ro",
            "--volume", f"{RUNTIME_DIR}:/compose_location",
            "--volume", f"{config_dir}:/config",
            "--volume", f"{data_dir}:/data",
            f"goharbor/prepare:{HARBOR_VERSION}", "prepare",
        ]
    )
    # The official proxy otherwise logs authorization-code callback URLs.
    nginx_config = config_dir / "nginx" / "nginx.conf"
    nginx_text = nginx_config.read_text(encoding="utf-8")
    nginx_text = re.sub(
        r"(?m)^(\s*)access_log\s+[^;]+;", r"\1access_log off;", nginx_text
    )
    nginx_config.write_text(nginx_text, encoding="utf-8")


def compose():
    return [
        "docker", "compose", "--project-name", COMPOSE_PROJECT,
        "-f", str(RUNTIME_DIR / "docker-compose.yml"),
        "-f", str(EXAMPLE_DIR / "docker-compose.override.yml"),
    ]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "action", choices=["prepare", "up", "down", "ps", "logs", "config"]
    )
    args = parser.parse_args()
    if args.action == "prepare":
        prepare()
        return
    if args.action == "up" and not (RUNTIME_DIR / "docker-compose.yml").exists():
        prepare()
    cmd = compose()
    if args.action == "up":
        run(cmd + ["config", "--quiet"])
        run(cmd + ["up", "-d"])
    elif args.action == "down":
        # Preserve database, keys and registry data; never use down -v here.
        run(cmd + ["down"])
    elif args.action == "ps":
        run(cmd + ["ps", "-a"])
    elif args.action == "logs":
        run(cmd + ["logs", "--tail", "50", "core", "jobservice"])
    else:
        run(cmd + ["config", "--quiet"])


if __name__ == "__main__":
    main()
