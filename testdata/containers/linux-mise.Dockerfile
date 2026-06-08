# Debian + mise pre-installed at /usr/local/bin/mise so `mise` is on
# PATH for non-login shells. Shim directory is added to PATH explicitly
# so `node` resolves once `mise install node@X` has run.
FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ca-certificates coreutils curl git \
    && rm -rf /var/lib/apt/lists/*

RUN curl --retry 5 --retry-delay 5 --retry-all-errors --retry-max-time 120 \
        -fsSL https://mise.run | sh \
    && cp /root/.local/bin/mise /usr/local/bin/mise

ENV PATH="/root/.local/share/mise/shims:/usr/local/bin:${PATH}"
ENV MISE_DATA_DIR="/root/.local/share/mise"

WORKDIR /work
CMD ["bash"]
