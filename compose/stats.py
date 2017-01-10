from __future__ import absolute_import
from __future__ import unicode_literals


class Stats(object):
    """
    Stats for a given container
    """

    def __init__(self, container, stats_data):
        self.container = container
        self.stats_data = stats_data

    def calculate_cpu_percent_unix(self):
        cpu_percent = 0.0
        # calculate the change for the cpu usage of the container in between readings
        cpu_delta = (self.stats_data["cpu_stats"]["cpu_usage"]["total_usage"]
                     - self.stats_data["precpu_stats"]["cpu_usage"]["total_usage"])
        # calculate the change for the entire system between readings
        system_delta = (self.stats_data["cpu_stats"]["system_cpu_usage"]
                        - self.stats_data["precpu_stats"]["system_cpu_usage"])
        if system_delta > 0.0 and cpu_delta > 0.0:
            num_cpus = len(self.stats_data["cpu_stats"]["cpu_usage"]["percpu_usage"])
            cpu_percent = (cpu_delta / float(system_delta)) * num_cpus * 100.0
        return cpu_percent

    def calculate_mem_usage(self):
        return self.stats_data["memory_stats"]["usage"]

    def calculate_mem_limit(self):
        return self.stats_data["memory_stats"]["limit"]

    def calculate_net_io(self):
        i, o = (0, 0)
        for network in self.stats_data["networks"]:
            i += self.stats_data["networks"][network]["rx_bytes"]
            o += self.stats_data["networks"][network]["tx_bytes"]
        return i, o

    def calculate_block_io(self):
        i, o = (0, 0)
        for entry in self.stats_data["blkio_stats"]["io_service_bytes_recursive"]:
            if entry["op"] == "Read":
                i += entry["value"]
            elif entry["op"] == "Write":
                o += entry["value"]
        return i, o
