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


install_requires = [
    'docker-py==0.2.3',
    'docopt==0.6.1',
    'PyYAML==3.10',
    'texttable==0.8.1',
    # unfortunately `docker` requires six ==1.3.0
    'six==1.3.0',
]


dev_requires = [
    'nose'
]


setup(
    name='fig',
    version=find_version("fig", "__init__.py"),
    description='Punctual, lightweight development environments using Docker',
    url='https://github.com/orchardup/fig',
    author='Orchard Laboratories Ltd.',
    author_email='hello@orchardup.com',
    packages=find_packages(),
    tests_require=['nose'],
    include_package_data=True,
    test_suite='nose.collector',
    install_requires=install_requires,
    entry_points="""
    [console_scripts]
    fig=fig.cli.main:main
    """,
)
