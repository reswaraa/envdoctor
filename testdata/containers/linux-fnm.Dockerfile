# Debian + fnm installed at /usr/local/bin/fnm. PATH includes the active
# node shim so `node --version` resolves once `fnm use X` has run.
FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ca-certificates coreutils curl unzip \
    && rm -rf /var/lib/apt/lists/*

# `fnm.vercel.app/install` is a redirect to a release zip on GitHub,
# which intermittently returns 504 under load. --retry flags let curl
# wait it out instead of failing the whole CI run. The installer
# script itself also calls curl; we set CURL options via env so the
# inner invocation gets the same retries.
ENV CURL_OPTS="--retry 5 --retry-delay 5 --retry-all-errors --retry-max-time 120"
RUN curl $CURL_OPTS -fsSL https://fnm.vercel.app/install \
    | bash -s -- --install-dir /usr/local/fnm --skip-shell \
    && ln -s /usr/local/fnm/fnm /usr/local/bin/fnm

ENV FNM_DIR="/root/.local/share/fnm"
ENV PATH="/root/.local/share/fnm/aliases/default/bin:${PATH}"

WORKDIR /work
CMD ["bash"]
