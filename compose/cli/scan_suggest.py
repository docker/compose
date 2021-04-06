import json
import logging
import os
from distutils.util import strtobool

from docker.constants import IS_WINDOWS_PLATFORM
from docker.utils.config import find_config_file


SCAN_BINARY_NAME = "docker-scan" + (".exe" if IS_WINDOWS_PLATFORM else "")

log = logging.getLogger(__name__)


class ScanConfig:
    def __init__(self, d):
        self.optin = False
        vars(self).update(d)


def display_scan_suggest_msg():
    if environment_scan_avoid_suggest() or \
            scan_available() is None or \
            scan_already_invoked():
        return
    log.info("Use 'docker scan' to run Snyk tests against images to find vulnerabilities "
             "and learn how to fix them")


def environment_scan_avoid_suggest():
    return os.getenv('DOCKER_SCAN_SUGGEST', 'true').lower() == 'false'


def scan_already_invoked():
    docker_folder = docker_config_folder()
    if docker_folder is None:
        return False

    scan_config_file = os.path.join(docker_folder, 'scan', "config.json")
    if not os.path.exists(scan_config_file):
        return False

    try:
        data = ''
        with open(scan_config_file) as f:
            data = f.read()
        scan_config = json.loads(data, object_hook=ScanConfig)
        return scan_config.optin if isinstance(scan_config.optin, bool) else strtobool(scan_config.optin)
    except Exception:  # pylint:disable=broad-except
        return True


def scan_available():
    docker_folder = docker_config_folder()
    if docker_folder:
        home_scan_bin = os.path.join(docker_folder, 'cli-plugins', SCAN_BINARY_NAME)
        if os.path.isfile(home_scan_bin) or os.path.islink(home_scan_bin):
            return home_scan_bin

    if IS_WINDOWS_PLATFORM:
        program_data_scan_bin = os.path.join('C:\\', 'ProgramData', 'Docker', 'cli-plugins',
                                             SCAN_BINARY_NAME)
        if os.path.isfile(program_data_scan_bin) or os.path.islink(program_data_scan_bin):
            return program_data_scan_bin
    else:
        lib_scan_bin = os.path.join('/usr', 'local', 'lib', 'docker',  'cli-plugins', SCAN_BINARY_NAME)
        if os.path.isfile(lib_scan_bin) or os.path.islink(lib_scan_bin):
            return lib_scan_bin
        lib_exec_scan_bin = os.path.join('/usr', 'local', 'libexec', 'docker',  'cli-plugins',
                                         SCAN_BINARY_NAME)
        if os.path.isfile(lib_exec_scan_bin) or os.path.islink(lib_exec_scan_bin):
            return lib_exec_scan_bin
        lib_scan_bin = os.path.join('/usr', 'lib', 'docker',  'cli-plugins', SCAN_BINARY_NAME)
        if os.path.isfile(lib_scan_bin) or os.path.islink(lib_scan_bin):
            return lib_scan_bin
        lib_exec_scan_bin = os.path.join('/usr', 'libexec', 'docker',  'cli-plugins', SCAN_BINARY_NAME)
        if os.path.isfile(lib_exec_scan_bin) or os.path.islink(lib_exec_scan_bin):
            return lib_exec_scan_bin
    return None


def docker_config_folder():
    docker_config_file = find_config_file()
    return None if not docker_config_file \
        else os.path.dirname(os.path.abspath(docker_config_file))
