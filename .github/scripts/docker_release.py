"""校验镜像版本，并决定正式版本能否更新 latest。"""

import argparse
import re
import subprocess


TAG_PATTERN = re.compile(
    r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)"
    r"(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?"
)


def parse_tag(tag):
    match = TAG_PATTERN.fullmatch(tag)
    if not match or len(tag) > 128:
        raise ValueError("expected a Docker-compatible tag vMAJOR.MINOR.PATCH[-PRERELEASE]")
    prerelease = match[4]
    if prerelease and any(
        part.isdigit() and len(part) > 1 and part.startswith("0")
        for part in prerelease.split(".")
    ):
        raise ValueError("numeric prerelease identifiers must not have leading zeros")
    return tuple(int(match[i]) for i in (1, 2, 3)), prerelease


def should_update_latest(tag, tags):
    version, prerelease = parse_tag(tag)
    if prerelease:
        return False
    for other in tags:
        try:
            other_version, other_prerelease = parse_tag(other)
        except ValueError:
            continue
        if not other_prerelease and other_version > version:
            return False
    return True


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag")
    parser.add_argument("--check-latest", action="store_true")
    args = parser.parse_args()
    try:
        parse_tag(args.tag)
    except ValueError as error:
        parser.error(str(error))
    print(f"version={args.tag[1:]}")
    if args.check_latest:
        # 发布锁内读取远端 tag，避免较慢的旧版本构建覆盖新版本 latest。
        result = subprocess.run(
            ["git", "ls-remote", "--tags", "--refs", "origin"],
            check=True, capture_output=True, text=True,
        )
        tags = [line.split("refs/tags/", 1)[1] for line in result.stdout.splitlines()]
        print(f"latest={str(should_update_latest(args.tag, tags)).lower()}")


if __name__ == "__main__":
    main()
