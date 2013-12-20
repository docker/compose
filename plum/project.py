from .service import Service

def sort_service_dicts(services):
    # Sort in dependency order
    def cmp(x, y):
        x_deps_y = y['name'] in x.get('links', [])
        y_deps_x = x['name'] in y.get('links', [])
        if x_deps_y and not y_deps_x:
            return 1
        elif y_deps_x and not x_deps_y:
            return -1
        return 0
    return sorted(services, cmp=cmp)

class Project(object):
    """
    A collection of services.
    """
    def __init__(self, name, services, client):
        self.name = name
        self.services = services
        self.client = client

    @classmethod
    def from_dicts(cls, name, service_dicts, client):
        """
        Construct a ServiceCollection from a list of dicts representing services.
        """
        project = cls(name, [], client)
        for service_dict in sort_service_dicts(service_dicts):
            # Reference links by object
            links = []
            if 'links' in service_dict:
                for service_name in service_dict.get('links', []):
                    links.append(project.get_service(service_name))
                del service_dict['links']
            project.services.append(Service(client=client, project=name, links=links, **service_dict))
        return project

    @classmethod
    def from_config(cls, name, config, client):
        dicts = []
        for service_name, service in config.items():
            service['name'] = service_name
            dicts.append(service)
        return cls.from_dicts(name, dicts, client)

    def get_service(self, name):
        for service in self.services:
            if service.name == name:
                return service

    def create_containers(self):
        """
        Returns a list of (service, container) tuples,
        one for each service with no running containers.
        """
        containers = []
        for service in self.services:
            if len(service.containers()) == 0:
                containers.append((service, service.create_container()))
        return containers

    def kill_and_remove(self, tuples):
        for (service, container) in tuples:
            container.kill()
            container.remove()

    def start(self, **options):
        for service in self.services:
            service.start(**options)

    def stop(self, **options):
        for service in self.services:
            service.stop(**options)

    def kill(self, **options):
        for service in self.services:
            service.kill(**options)

    def remove_stopped(self, **options):
        for service in self.services:
            service.remove_stopped(**options)

    def containers(self, *args, **kwargs):
        l = []
        for service in self.services:
            for container in service.containers(*args, **kwargs):
                l.append(container)
        return l


