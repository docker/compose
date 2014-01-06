#!/usr/bin/env python
# -*- coding: utf-8 -*-
from __future__ import unicode_literals
from __future__ import absolute_import
from setuptools import setup, find_packages
import re
import os
import codecs


def read(*parts):
    path = os.path.join(os.path.dirname(__file__), *parts)
    with codecs.open(path, encoding='utf-8') as fobj:
        return fobj.read()


def find_version(*file_paths):
    version_file = read(*file_paths)
    version_match = re.search(r"^__version__ = ['\"]([^'\"]*)['\"]",
                              version_file, re.M)
    if version_match:
        return version_match.group(1)
    raise RuntimeError("Unable to find version string.")

with open('requirements.txt') as f:
    install_requires = f.read().splitlines()

with open('requirements-dev.txt') as f:
    tests_require = f.read().splitlines()

setup(
    name='fig',
    version=find_version("fig", "__init__.py"),
    description='Punctual, lightweight development environments using Docker',
    url='https://github.com/orchardup/fig',
    author='Orchard Laboratories Ltd.',
    author_email='hello@orchardup.com',
    packages=find_packages(),
    include_package_data=True,
    test_suite='nose.collector',
    install_requires=install_requires,
    tests_require=tests_require,
    entry_points="""
    [console_scripts]
    fig=fig.cli.main:main
    """,
)
