# Release process

Releases are maintainer-driven and use semantic versions. Do not publish a
release until the relevant security gate and CI are green.

1. Start from a clean `main` synchronized with GitHub.
2. Choose `MAJOR.MINOR.PATCH` and update the same value in:
   - `noctalia-plugin/waypass/plugin.toml`;
   - `packaging/arch/PKGBUILD`;
   - `packaging/arch/build-local-package.sh`.
3. Move the release notes from `Unreleased` to a dated section in
   `CHANGELOG.md`, and update its comparison links.
4. Run the full gate from `CONTRIBUTING.md` and build the Arch package in a
   clean environment. Use only synthetic password-store data.
5. Review the release commit, then create a signed annotated `vMAJOR.MINOR.PATCH`
   tag. Push the commit and tag explicitly.
6. Create a GitHub release from the tag and verify its generated notes against
   `CHANGELOG.md`. Do not attach locally built binaries until a documented,
   reproducible and attestable artifact workflow exists.

A Noctalia community submission is a separate process: copy only the plugin
directory, satisfy the current community checklist and bump the plugin version
for every submitted change.
