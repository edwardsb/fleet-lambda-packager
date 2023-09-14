FROM fleetdm/fleetctl
RUN mkdir -p /tmp/build
COPY packager /opt/packager
RUN chmod +x /opt/packager
WORKDIR /tmp
ENTRYPOINT ["/opt/packager"]