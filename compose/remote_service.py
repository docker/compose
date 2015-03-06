class RemoteService(object):
    def __init__(
        self,
        name,
        host,
        client=None,
        project='default',
        port=None,
        environment=None
    ):
        self.name = name
        self.client = client
        self.project = project
        self.host = host
        self.port = port
        self.environment = environment

    def containers(self):
        return []

    def link_environment_variables(self):
        env = {}

        if self.environment is not None:
            for (key, value) in self.environment.items():
                env["ENV_%s" % key] = value

        if self.port is not None:
            env["PORT"] = "tcp://%s:%s" % (self.host, self.port)
            env["PORT_%s_TCP" % self.port] = "tcp://%s:%s" % (self.host, self.port)
            env["PORT_%s_TCP_ADDR" % self.port] = self.host
            env["PORT_%s_TCP_PORT" % self.port] = self.port
            env["PORT_%s_TCP_PROTO" % self.port] = "tcp"

        return env

    def link_hostnames(self):
        return [self.host]
