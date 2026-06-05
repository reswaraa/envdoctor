# Debian + fnm installed at /usr/local/bin/fnm. PATH includes the active
# node shim so `node --version` resolves once `fnm use X` has run.
FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ca-certificates coreutils curl unzip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://fnm.vercel.app/install | bash -s -- --install-dir /usr/local/fnm --skip-shell \
    && ln -s /usr/local/fnm/fnm /usr/local/bin/fnm

ENV FNM_DIR="/root/.local/share/fnm"
ENV PATH="/root/.local/share/fnm/aliases/default/bin:${PATH}"

WORKDIR /work
CMD ["bash"]
