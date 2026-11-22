### General

- The companion hands its per-launch secret to its server over standard input, not the environment, so another program of yours can no longer read it with `ps` and approve its own requests (#3927).
