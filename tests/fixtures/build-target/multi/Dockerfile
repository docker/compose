FROM busybox:1.27.2
RUN true

FROM busybox:1.27.2 as testpoint
LABEL com.docker.compose.test_image=true

FROM busybox:1.27.2 as too_far
RUN true
CMD echo "success"
