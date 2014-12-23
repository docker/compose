from cli import docker_client

cluster_mode = None

def find_container_name(names):
    for name in names:
        values = name.split('/')
        if is_cluster_mode():
            if len(values) == 3:
                return values[2]
        else:
            if len(values) == 2:
                return values[1]

def find_container_name_with_host(names):
    n = 2
    if is_cluster_mode():
        n = 3
    for name in names:
        if len(name.split('/')) == n:
            return name

def get_container_name_without_host(name):
    names = name.split('/')
    return names[len(names) - 1]


def is_cluster_mode():
    global cluster_mode
    if cluster_mode == None:
        cluster_mode = False
        info = docker_client.docker_client().info()
        driverStatus = info['DriverStatus']
        for status in driverStatus:
            if isinstance(status, list) and status[0] == u'\x08Nodes':
                cluster_mode = True
                break
    return cluster_mode

