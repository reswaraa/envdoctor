# Minimal Debian environment used by recipes whose `test:` block only
# needs basic shell utilities (env-required's cp, port-free's lsof+kill).
# No language version manager installed; recipes that need one reference
# a different container fixture.
FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ca-certificates coreutils curl lsof procps netcat-openbsd \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /work
CMD ["bash"]
