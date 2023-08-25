FROM fleetdm/fleetctl
WORKDIR /opt/packager
RUN mkdir "build"
COPY packager .
RUN chmod +x packager
ENTRYPOINT ["./packager"]