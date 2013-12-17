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

class ServiceCollection(list):
    @classmethod
    def from_dicts(cls, client, service_dicts):
        """
        Construct a ServiceCollection from a list of dicts representing services.
        """
        collection = ServiceCollection()
        for service_dict in sort_service_dicts(service_dicts):
            # Reference links by object
            links = []
            if 'links' in service_dict:
                for name in service_dict.get('links', []):
                    links.append(collection.get(name))
                del service_dict['links']
            collection.append(Service(client=client, links=links, **service_dict))
        return collection

    @classmethod
    def from_config(cls, client, config):
        dicts = []
        for name, service in config.items():
            service['name'] = name
            dicts.append(service)
        return cls.from_dicts(client, dicts)

    def get(self, name):
        for service in self:
            if service.name == name:
                return service

    def start(self):
        for service in self:
            service.start()

    def stop(self):
        for service in self:
            service.stop()



