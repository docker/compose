import logging
import re

from .container import get_container_name, Container


log = logging.getLogger(__name__)


# TODO: remove this section when migrate_project_to_labels is removed
NAME_RE = re.compile(r'^([^_]+)_([^_]+)_(run_)?(\d+)$')


def is_valid_name(name):
    match = NAME_RE.match(name)
    return match is not None


def check_for_legacy_containers(
        client,
        project,
        services,
        stopped=False,
        one_off=False):
    """Check if there are containers named using the old naming convention
    and warn the user that those containers may need to be migrated to
    using labels, so that compose can find them.
    """
    for container in client.containers(all=stopped):
        name = get_container_name(container)
        for service in services:
            prefix = '%s_%s_%s' % (project, service, 'run_' if one_off else '')
            if not name.startswith(prefix):
                continue

            log.warn(
                "Compose found a found a container named %s without any "
                "labels. As of compose 1.3.0 containers are identified with "
                "labels instead of naming convention. If you'd like compose "
                "to use this container, please run "
                "`docker-compose migrate-to-labels`" % (name,))


def add_labels(project, container, name):
    project_name, service_name, one_off, number = NAME_RE.match(name).groups()
    if project_name != project.name or service_name not in project.service_names:
        return
    service = project.get_service(service_name)
    service.recreate_container(container)


def migrate_project_to_labels(project):
    log.info("Running migration to labels for project %s", project.name)

    client = project.client
    for container in client.containers(all=True):
        name = get_container_name(container)
        if not is_valid_name(name):
            continue
        add_labels(project, Container.from_ps(client, container), name)
