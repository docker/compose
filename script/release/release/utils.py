from __future__ import absolute_import
from __future__ import unicode_literals

import os
import re

from .const import REPO_ROOT
from compose import const as compose_const

section_header_re = re.compile(r'^[0-9]+\.[0-9]+\.[0-9]+ \([0-9]{4}-[01][0-9]-[0-3][0-9]\)$')


class ScriptError(Exception):
    pass


def branch_name(version):
    return 'bump-{}'.format(version)


def read_release_notes_from_changelog():
    with open(os.path.join(REPO_ROOT, 'CHANGELOG.md'), 'r') as f:
        lines = f.readlines()
    i = 0
    while i < len(lines):
        if section_header_re.match(lines[i]):
            break
        i += 1

    j = i + 1
    while j < len(lines):
        if section_header_re.match(lines[j]):
            break
        j += 1

    return ''.join(lines[i + 2:j - 1])


def update_init_py_version(version):
    path = os.path.join(REPO_ROOT, 'compose', '__init__.py')
    with open(path, 'r') as f:
        contents = f.read()
    contents = re.sub(r"__version__ = '[0-9a-z.-]+'", "__version__ = '{}'".format(version), contents)
    with open(path, 'w') as f:
        f.write(contents)


def update_run_sh_version(version):
    path = os.path.join(REPO_ROOT, 'script', 'run', 'run.sh')
    with open(path, 'r') as f:
        contents = f.read()
    contents = re.sub(r'VERSION="[0-9a-z.-]+"', 'VERSION="{}"'.format(version), contents)
    with open(path, 'w') as f:
        f.write(contents)


def compatibility_matrix():
    result = {}
    for engine_version in compose_const.API_VERSION_TO_ENGINE_VERSION.values():
        result[engine_version] = []
    for fmt, api_version in compose_const.API_VERSIONS.items():
        result[compose_const.API_VERSION_TO_ENGINE_VERSION[api_version]].append(fmt.vstring)
    return result


def yesno(prompt, default=None):
    """
    Prompt the user for a yes or no.

    Can optionally specify a default value, which will only be
    used if they enter a blank line.

    Unrecognised input (anything other than "y", "n", "yes",
    "no" or "") will return None.
    """
    answer = input(prompt).strip().lower()

    if answer == "y" or answer == "yes":
        return True
    elif answer == "n" or answer == "no":
        return False
    elif answer == "":
        return default
    else:
        return None
