---
title: pipx
summary: >-
  `pipx install mdsmith` puts the PyPI wheel's
  `mdsmith` console script on PATH inside its own
  isolated virtualenv.
mechanism: toolchain
artifact: cli
command: pipx install mdsmith
audience: Isolated CLI install on Python hosts
platforms: [python]
channelurl: https://pypi.org/project/mdsmith/
weight: 6
---
# pipx

Release page: <https://pypi.org/project/mdsmith/>

`pipx` installs the mdsmith wheel into its own
isolated virtual environment. The `mdsmith` console
script then lands on the user's PATH. The install
touches no other Python project. Upgrade with
`pipx upgrade mdsmith`.
