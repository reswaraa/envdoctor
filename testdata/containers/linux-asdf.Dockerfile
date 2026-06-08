# Debian + asdf 0.15.x (the last "shell-script" major before asdf 0.16
# switched to a Go binary). Sourced into the shell via /etc/profile.d so
# non-interactive `bash -c` invocations still see `asdf`.
FROM debian:12-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        bash ca-certificates coreutils curl git build-essential \
    && rm -rf /var/lib/apt/lists/*

# --depth=1 keeps the clone small; git itself retries internally on
# transient network failures.
RUN git clone --depth=1 -b v0.15.0 https://github.com/asdf-vm/asdf.git /root/.asdf \
    && echo '. /root/.asdf/asdf.sh' > /etc/profile.d/asdf.sh \
    && echo 'export PATH="$HOME/.asdf/shims:$HOME/.asdf/bin:$PATH"' >> /etc/profile.d/asdf.sh

ENV BASH_ENV="/etc/profile.d/asdf.sh"
ENV PATH="/root/.asdf/shims:/root/.asdf/bin:${PATH}"

WORKDIR /work
CMD ["bash"]
