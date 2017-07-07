from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import os
import subprocess
import sys

# Attempt to detect https://github.com/docker/compose/issues/4344
try:
    # We don't try importing pip because it messes with package imports
    # on some Linux distros (Ubuntu, Fedora)
    # https://github.com/docker/compose/issues/4425
    # https://github.com/docker/compose/issues/4481
    # https://github.com/pypa/pip/blob/master/pip/_vendor/__init__.py
    env = os.environ.copy()
    env[str('PIP_DISABLE_PIP_VERSION_CHECK')] = str('1')

    s_cmd = subprocess.Popen(
        # DO NOT replace this call with a `sys.executable` call. It breaks the binary
        # distribution (with the binary calling itself recursively over and over).
        ['pip', 'freeze'], stderr=subprocess.PIPE, stdout=subprocess.PIPE,
        env=env
    )
    packages = s_cmd.communicate()[0].splitlines()
    dockerpy_installed = len(
        list(filter(lambda p: p.startswith(b'docker-py=='), packages))
    ) > 0
    if dockerpy_installed:
        from .colors import yellow
        print(
            yellow('WARNING:'),
            "Dependency conflict: an older version of the 'docker-py' package "
            "may be polluting the namespace. "
            "If you're experiencing crashes, run the following command to remedy the issue:\n"
            "pip uninstall docker-py; pip uninstall docker; pip install docker",
            file=sys.stderr
        )

except OSError:
    # pip command is not available, which indicates it's probably the binary
    # distribution of Compose which is not affected
    pass
except UnicodeDecodeError:
    # ref: https://github.com/docker/compose/issues/4663
    # This could be caused by a number of things, but it seems to be a
    # python 2 + MacOS interaction. It's not ideal to ignore this, but at least
    # it doesn't make the program unusable.
    pass
