FROM flml/flashlight:cuda-latest@sha256:42ccb7981aa4edaa1d8881ce9711583d046d00db2d80049bf7114e1441417cf9

RUN apt-get update && \
    apt-get install --yes --no-install-recommends ffmpeg && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/* /tmp/*

COPY bin/server /usr/local/bin/
COPY config.yml /

WORKDIR /
CMD [ "/usr/local/bin/server", "-config", "config.yml" ]
