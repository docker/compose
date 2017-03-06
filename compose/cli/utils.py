from __future__ import absolute_import
from __future__ import division
from __future__ import unicode_literals

import math
import os
import platform
import ssl
import subprocess
import sys

import docker

import compose
from ..const import IS_WINDOWS_PLATFORM

# WindowsError is not defined on non-win32 platforms. Avoid runtime errors by
# defining it as OSError (its parent class) if missing.
try:
    WindowsError
except NameError:
    WindowsError = OSError


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


def input(prompt):
    """
    Version of input (raw_input in Python 2) which forces a flush of sys.stdout
    to avoid problems where the prompt fails to appear due to line buffering
    """
    sys.stdout.write(prompt)
    sys.stdout.flush()
    return sys.stdin.readline().rstrip('\n')


def call_silently(*args, **kwargs):
    """
    Like subprocess.call(), but redirects stdout and stderr to /dev/null.
    """
    with open(os.devnull, 'w') as shutup:
        try:
            return subprocess.call(*args, stdout=shutup, stderr=shutup, **kwargs)
        except WindowsError:
            # On Windows, subprocess.call() can still raise exceptions. Normalize
            # to POSIXy behaviour by returning a nonzero exit code.
            return 1


def is_mac():
    return platform.system() == 'Darwin'


def is_ubuntu():
    return platform.system() == 'Linux' and platform.linux_distribution()[0] == 'Ubuntu'


def is_windows():
    return IS_WINDOWS_PLATFORM


def get_version_info(scope):
    versioninfo = 'docker-compose version {}, build {}'.format(
        compose.__version__,
        get_build_version())

    if scope == 'compose':
        return versioninfo
    if scope == 'full':
        return (
            "{}\n"
            "docker-py version: {}\n"
            "{} version: {}\n"
            "OpenSSL version: {}"
        ).format(
            versioninfo,
            docker.version,
            platform.python_implementation(),
            platform.python_version(),
            ssl.OPENSSL_VERSION)

    raise ValueError("{} is not a valid version scope".format(scope))


def get_build_version():
    filename = os.path.join(os.path.dirname(compose.__file__), 'GITSHA')
    if not os.path.exists(filename):
        return 'unknown'

    with open(filename) as fh:
        return fh.read().strip()


def is_docker_for_mac_installed():
    return is_mac() and os.path.isdir('/Applications/Docker.app')


def generate_user_agent():
    parts = [
        "docker-compose/{}".format(compose.__version__),
        "docker-py/{}".format(docker.__version__),
    ]
    try:
        p_system = platform.system()
        p_release = platform.release()
    except IOError:
        pass
    else:
        parts.append("{}/{}".format(p_system, p_release))
    return " ".join(parts)


def unquote_path(s):
    if not s:
        return s
    if s[0] == '"' and s[-1] == '"':
        return s[1:-1]
    return s


def human_readable_file_size(size):
    suffixes = ['B', 'kB', 'MB', 'GB', 'TB', 'PB', 'EB', ]
    order = int(math.log(size, 2) / 10) if size else 0
    if order >= len(suffixes):
        order = len(suffixes) - 1

    return '{0:.3g} {1}'.format(
        size / float(1 << (order * 10)),
        suffixes[order]
    )
