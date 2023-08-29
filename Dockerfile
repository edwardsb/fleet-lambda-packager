FROM fleetdm/fleetctl
WORKDIR /tmp
RUN mkdir "build"
COPY packager .
RUN chmod +x packager
ENTRYPOINT ["./packager"]