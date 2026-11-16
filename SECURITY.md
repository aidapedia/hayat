# Security policy

Hayat signs people in and decides what they may do, so we take reports
seriously and handle them before anything else.

## Reporting a vulnerability

Please **don't open a public issue**. Email
**kurniaji.gunawan@aidapedia.com** with:

- what you found and what an attacker could do with it,
- steps to reproduce, or a proof of concept,
- the version (`hayat version`) or commit, and which role is affected.

You'll get an acknowledgement within a few days. We'll keep you posted while we
work on a fix, and credit you in the release notes unless you'd rather we
didn't.

## Supported versions

Fixes land on the latest release. Run the newest image
(`ghcr.io/aidapedia/hayat:latest`, or the newest version tag) to get them.

## Scope

Anything that lets someone sign in as another person, read or change what they
shouldn't, bypass a decision, forge or replay a token, or read secrets (the
master key, signing keys, client secrets, password hashes) is in scope.

Deployments running with `HAYAT_DEV=true` relax security rules on purpose and
are out of scope.
