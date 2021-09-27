#!/usr/bin/env python3
import glob
import os.path
import re
import subprocess

USAGE_RE = re.compile(r"```.*?\nUsage:.*?```", re.MULTILINE | re.DOTALL)
USAGE_IN_CMD_RE = re.compile(r"^Usage:.*", re.MULTILINE | re.DOTALL)

HELP_CMD = "docker run --rm docker/compose:latest %s --help"

for file in glob.glob("compose/reference/*.md"):
    with open(file) as f:
        data = f.read()
    if not USAGE_RE.search(data):
        print("Not a command:", file)
        continue
    subcmd = os.path.basename(file).replace(".md", "")
    if subcmd == "overview":
        continue
    print(f"Found {subcmd}: {file}")
    help_cmd = HELP_CMD % subcmd
    help = subprocess.check_output(help_cmd.split())
    help = help.decode("utf-8")
    help = USAGE_IN_CMD_RE.findall(help)[0]
    help = help.strip()
    data = USAGE_RE.sub(f"```none\n{help}\n```", data)
    with open(file, "w") as f:
        f.write(data)
