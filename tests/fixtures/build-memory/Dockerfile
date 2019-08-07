FROM busybox:1.31.0-uclibc

# Report the memory (through the size of the group memory)
RUN echo "memory:" $(cat /sys/fs/cgroup/memory/memory.limit_in_bytes)
