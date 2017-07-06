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
        [
            sys.executable, '-c',
            """import pkg_resources as pr; print "\\n".join([d.project_name for d in pr.working_set])"""
        ], stderr=subprocess.PIPE, stdout=subprocess.PIPE,
        env=env
    )
    packages = s_cmd.communicate()[0].splitlines()
    if 'docker-py' in packages:
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
    # python command is not available, which indicates it's probably the binary
    # distribution of Compose which is not affected
    pass
except UnicodeDecodeError:
    # ref: https://github.com/docker/compose/issues/4663
    # This could be caused by a number of things, but it seems to be a
    # python 2 + MacOS interaction. It's not ideal to ignore this, but at least
    # it doesn't make the program unusable.
    pass
