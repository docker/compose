#!/usr/bin/env python
# -*- coding: utf-8 -*-
from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import codecs
import os
import re
import sys

import pkg_resources
from setuptools import find_packages
from setuptools import setup


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
    'cached-property >= 1.2.0, < 2',
    'docopt >= 0.6.1, < 0.7',
    'PyYAML >= 3.10, < 4',
    'requests >= 2.6.1, != 2.11.0, < 2.12',
    'texttable >= 0.9.0, < 0.10',
    'websocket-client >= 0.32.0, < 1.0',
    'docker >= 2.5.1, < 3.0',
    'dockerpty >= 0.4.1, < 0.5',
    'six >= 1.3.0, < 2',
    'jsonschema >= 2.5.1, < 3',
]


tests_require = [
    'pytest',
]


if sys.version_info[:2] < (3, 4):
    tests_require.append('mock >= 1.0.1')

extras_require = {
    ':python_version < "3.4"': ['enum34 >= 1.0.4, < 2'],
    ':python_version < "3.5"': ['backports.ssl_match_hostname >= 3.5'],
    ':python_version < "3.3"': ['ipaddress >= 1.0.16'],
    ':sys_platform == "win32"': ['colorama >= 0.3.7, < 0.4'],
    'socks': ['PySocks >= 1.5.6, != 1.5.7, < 2'],
}


try:
    if 'bdist_wheel' not in sys.argv:
        for key, value in extras_require.items():
            if key.startswith(':') and pkg_resources.evaluate_marker(key[1:]):
                install_requires.extend(value)
except Exception as e:
    print("Failed to compute platform dependencies: {}. ".format(e) +
          "All dependencies will be installed as a result.", file=sys.stderr)
    for key, value in extras_require.items():
        if key.startswith(':'):
            install_requires.extend(value)


setup(
    name='docker-compose',
    version=find_version("compose", "__init__.py"),
    description='Multi-container orchestration for Docker',
    url='https://www.docker.com/',
    author='Docker, Inc.',
    license='Apache License 2.0',
    packages=find_packages(exclude=['tests.*', 'tests']),
    include_package_data=True,
    test_suite='nose.collector',
    install_requires=install_requires,
    extras_require=extras_require,
    tests_require=tests_require,
    entry_points="""
    [console_scripts]
    docker-compose=compose.cli.main:main
    """,
    classifiers=[
        'Development Status :: 5 - Production/Stable',
        'Environment :: Console',
        'Intended Audience :: Developers',
        'License :: OSI Approved :: Apache Software License',
        'Programming Language :: Python :: 2',
        'Programming Language :: Python :: 2.7',
        'Programming Language :: Python :: 3',
        'Programming Language :: Python :: 3.4',
    ],
)
