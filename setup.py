#!/usr/bin/env python
# -*- coding: utf-8 -*-

from setuptools import setup
import re
import os
import codecs


# Borrowed from
# https://github.com/jezdez/django_compressor/blob/develop/setup.py
def read(*parts):
    return codecs.open(os.path.join(os.path.dirname(__file__), *parts)).read()


def find_version(*file_paths):
    version_file = read(*file_paths)
    version_match = re.search(r"^__version__ = ['\"]([^'\"]*)['\"]",
                              version_file, re.M)
    if version_match:
        return version_match.group(1)
    raise RuntimeError("Unable to find version string.")

with open('requirements.txt') as f:
    install_requires = f.read().splitlines()

setup(
    name='fig',
    version=find_version("fig", "__init__.py"),
    description='Punctual, lightweight development environments using Docker',
    url='https://github.com/orchardup/fig',
    author='Orchard Laboratories Ltd.',
    author_email='hello@orchardup.com',
    packages=['fig', 'fig.cli'],
    package_data={},
    include_package_data=True,
    install_requires=install_requires,
    entry_points="""
    [console_scripts]
    fig=fig.cli.main:main
    """,
)
