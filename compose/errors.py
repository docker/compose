class OperationFailedError(Exception):
    def __init__(self, reason):
        self.msg = reason


class StreamParseError(RuntimeError):
    def __init__(self, reason):
        self.msg = reason


class HealthCheckException(Exception):
    def __init__(self, reason):
        self.msg = reason


class HealthCheckFailed(HealthCheckException):
    def __init__(self, container_id):
        super().__init__(
            'Container "{}" is unhealthy.'.format(container_id)
        )


class NoHealthCheckConfigured(HealthCheckException):
    def __init__(self, service_name):
        super().__init__(
            'Service "{}" is missing a healthcheck configuration'.format(
                service_name
            )
        )
