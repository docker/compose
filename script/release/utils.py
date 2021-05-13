import os
import re

from const import REPO_ROOT


def update_init_py_version(version):
    path = os.path.join(REPO_ROOT, 'compose', '__init__.py')
    with open(path) as f:
        contents = f.read()
    contents = re.sub(r"__version__ = '[0-9a-z.-]+'", "__version__ = '{}'".format(version), contents)
    with open(path, 'w') as f:
        f.write(contents)


def update_run_sh_version(version):
    path = os.path.join(REPO_ROOT, 'script', 'run', 'run.sh')
    with open(path) as f:
        contents = f.read()
    contents = re.sub(r'VERSION="[0-9a-z.-]+"', 'VERSION="{}"'.format(version), contents)
    with open(path, 'w') as f:
        f.write(contents)


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
