import hashlib
import json
import logging
import os

import concurrent.futures

from .const import DEFAULT_MAX_WORKERS


log = logging.getLogger(__name__)


def parallel_execute(command, containers, doing_msg, done_msg, **options):
    """
    Execute a given command upon a list of containers in parallel.
    """
    max_workers = os.environ.get('COMPOSE_MAX_WORKERS', DEFAULT_MAX_WORKERS)

    def container_command_execute(container, command, **options):
        log.info("{} {}...".format(doing_msg, container.name))
        return getattr(container, command)(**options)

    with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
        future_container = {
            executor.submit(
                container_command_execute,
                container,
                command,
                **options
            ): container for container in containers
        }

        for future in concurrent.futures.as_completed(future_container):
            container = future_container[future]
            log.info("{} {}".format(done_msg, container.name))


def json_hash(obj):
    dump = json.dumps(obj, sort_keys=True, separators=(',', ':'))
    h = hashlib.sha256()
    h.update(dump)
    return h.hexdigest()
