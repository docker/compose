#!/usr/bin/env python
"""
Compute a file checksum.
"""
from __future__ import absolute_import
from __future__ import unicode_literals

import argparse
import hashlib
import os


def checksum(input, output=None):
    m = hashlib.sha256()
    with open(input, 'rb') as f:
        for chunk in iter(lambda: f.read(4096), b""):
            m.update(chunk)

    outputStr = '{}  {}\n'.format(m.hexdigest(), os.path.basename(input))
    if output is None:
        print(outputStr)
    else:
        with open(output, 'w') as f:
            f.write(outputStr)


def parse_args(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('-i', '--input', help="Input file")
    parser.add_argument('-o', '--output', help="Ouput destination or - to print to stdout")
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    checksum(args.input, args.output)


if __name__ == "__main__":
    main()
