from __future__ import absolute_import
from __future__ import print_function
from __future__ import unicode_literals

import subprocess
import sys

# Attempt to detect https://github.com/docker/compose/issues/4344
try:
    # We don't try importing pip because it messes with package imports
    # on some Linux distros (Ubuntu, Fedora)
    # https://github.com/docker/compose/issues/4425
    # https://github.com/docker/compose/issues/4481
    # https://github.com/pypa/pip/blob/master/pip/_vendor/__init__.py
    s_cmd = subprocess.Popen(
        ['pip', 'freeze'], stderr=subprocess.PIPE, stdout=subprocess.PIPE
    )
    packages = s_cmd.communicate()[0].splitlines()
    dockerpy_installed = len(
        list(filter(lambda p: p.startswith(b'docker-py=='), packages))
    ) > 0
    if dockerpy_installed:
        from .colors import red
        print(
            red('ERROR:'),
            "Dependency conflict: an older version of the 'docker-py' package "
            "is polluting the namespace. "
            "Run the following command to remedy the issue:\n"
            "pip uninstall docker docker-py; pip install docker",
            file=sys.stderr
        )
        sys.exit(1)

except OSError:
    # pip command is not available, which indicates it's probably the binary
    # distribution of Compose which is not affected
    pass
