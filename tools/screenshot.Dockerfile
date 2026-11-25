FROM golang:1.27.4

RUN apt-get update \
    && apt-get install --yes --no-install-recommends chromium curl fonts-dejavu-core imagemagick python3 \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
ENTRYPOINT ["sh", "tools/capture-screenshot.sh"]
