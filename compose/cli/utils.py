from __future__ import absolute_import
from __future__ import division
from __future__ import unicode_literals

import os
import platform
import ssl
import subprocess

from docker import version as docker_py_version
from six.moves import input

from .. import __version__


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


def get_version_info(scope):
    versioninfo = 'docker-compose version: %s' % __version__
    if scope == 'compose':
        return versioninfo
    elif scope == 'full':
        return versioninfo + '\n' \
            + "docker-py version: %s\n" % docker_py_version \
            + "%s version: %s\n" % (platform.python_implementation(), platform.python_version()) \
            + "OpenSSL version: %s" % ssl.OPENSSL_VERSION
    else:
        raise RuntimeError('passed unallowed value to `cli.utils.get_version_info`')
