from __future__ import absolute_import
from __future__ import unicode_literals

import logging
import os

from .errors import ComposeFileNotFound
from .errors import DuplicateOverrideFileFound

log = logging.getLogger(__name__)

DEFAULT_OVERRIDE_FILENAMES = ('docker-compose.override.yml', 'docker-compose.override.yaml')


def find(filenames, base_dir):
    (candidates, path) = find_candidates_in_parent_dirs(filenames, base_dir)

    if not candidates:
        raise ComposeFileNotFound(filenames)

    winner = candidates[0]

    if len(candidates) > 1:
        log.warn("Found multiple config files with supported names: %s", ", ".join(candidates))
        log.warn("Using %s\n", winner)

    return [os.path.join(path, winner)] + get_default_override_file(path)


def get_default_override_file(path):
    override_files_in_path = [os.path.join(path, override_filename) for override_filename
                              in DEFAULT_OVERRIDE_FILENAMES
                              if os.path.exists(os.path.join(path, override_filename))]
    if len(override_files_in_path) > 1:
        raise DuplicateOverrideFileFound(override_files_in_path)
    return override_files_in_path


def find_candidates_in_parent_dirs(filenames, path):
    """
    Given a directory path to start, looks for filenames in the
    directory, and then each parent directory successively,
    until found.

    Returns tuple (candidates, path).
    """
    candidates = [filename for filename in filenames
                  if os.path.exists(os.path.join(path, filename))]

    if not candidates:
        parent_dir = os.path.join(path, '..')
        if os.path.abspath(parent_dir) != os.path.abspath(path):
            return find_candidates_in_parent_dirs(filenames, parent_dir)

    return (candidates, path)
