from compose.config.errors import DependencyError


def get_service_name_from_network_mode(network_mode):
    return get_source_name_from_network_mode(network_mode, 'service')


def get_container_name_from_network_mode(network_mode):
    return get_source_name_from_network_mode(network_mode, 'container')


def get_source_name_from_network_mode(network_mode, source_type):
    if not network_mode:
        return

    if not network_mode.startswith(source_type+':'):
        return

    _, net_name = network_mode.split(':', 1)
    return net_name


def get_service_names(links):
    return [link.split(':', 1)[0] for link in links]


def get_service_names_from_volumes_from(volumes_from):
    return [volume_from.source for volume_from in volumes_from]


def get_service_dependents(service_dict, services):
    name = service_dict['name']
    return [
        service for service in services
        if (name in get_service_names(service.get('links', [])) or
            name in get_service_names_from_volumes_from(service.get('volumes_from', [])) or
            name == get_service_name_from_network_mode(service.get('network_mode')) or
            name == get_service_name_from_network_mode(service.get('pid')) or
            name == get_service_name_from_network_mode(service.get('ipc')) or
            name in service.get('depends_on', []))
    ]


def sort_service_dicts(services):
    # Topological sort (Cormen/Tarjan algorithm).
    unmarked = services[:]
    temporary_marked = set()
    sorted_services = []

    def visit(n):
        if n['name'] in temporary_marked:
            if n['name'] in get_service_names(n.get('links', [])):
                raise DependencyError('A service can not link to itself: %s' % n['name'])
            if n['name'] in n.get('volumes_from', []):
                raise DependencyError('A service can not mount itself as volume: %s' % n['name'])
            if n['name'] in n.get('depends_on', []):
                raise DependencyError('A service can not depend on itself: %s' % n['name'])
            raise DependencyError('Circular dependency between %s' % ' and '.join(temporary_marked))

        if n in unmarked:
            temporary_marked.add(n['name'])
            for m in get_service_dependents(n, services):
                visit(m)
            temporary_marked.remove(n['name'])
            unmarked.remove(n)
            sorted_services.insert(0, n)

    while unmarked:
        visit(unmarked[-1])

    return sorted_services
