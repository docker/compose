FROM busybox

# Report the shm_size (through the size of /dev/shm)
RUN echo "shm_size:" $(df -h /dev/shm | tail -n 1 | awk '{print $2}')
