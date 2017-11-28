from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import subprocess
import sys
from ast import literal_eval as make_tuple

# Attempt to detect https://github.com/docker/compose/issues/4344
try:
    s_cmd = subprocess.Popen(
        # DO NOT replace this call with a `sys.executable` call. It breaks the binary
        # distribution (with the binary calling itself recursively over and over).
        [
            'python',
            '-c',
            'from __future__ import print_function; from docker import version_info; print(version_info)'
        ],
        stderr=subprocess.PIPE, stdout=subprocess.PIPE
    )

    stdout, stderr = s_cmd.communicate()

    # only check docker-py version if it's installed
    if not stderr:
        version = make_tuple(stdout.decode('utf-8'))
        # version 1.10.6 was the latest release of docker python client released as docker-py
        if version <= (1, 10, 6):
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
    # python binary is not available, which indicates it's probably the binary
    # distribution of Compose which is not affected
    pass
except UnicodeDecodeError:
    # ref: https://github.com/docker/compose/issues/4663
    # This could be caused by a number of things, but it seems to be a
    # python 2 + MacOS interaction. It's not ideal to ignore this, but at least
    # it doesn't make the program unusable.
    pass
