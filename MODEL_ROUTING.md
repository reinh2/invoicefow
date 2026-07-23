# Model and Agent Routing

This document is advisory. Do not pin unavailable model identifiers in the repository.

## Use stronger reasoning for

- initial architecture;
- transaction and concurrency design;
- job leasing and idempotency;
- file/process security;
- prompt-injection and trust boundaries;
- major refactors;
- final release review.

## Use a normal implementation model for

- handlers after contracts are fixed;
- repository methods;
- frontend screens;
- adapters following existing interfaces;
- migrations following an approved schema;
- integration tests.

## Use faster/cheaper subagents for

- codebase mapping;
- locating files and symbols;
- documentation synchronization;
- fixtures;
- routine unit tests;
- lint cleanup;
- mechanical UI components.

## Important

A complex project is not one indivisible complex request. The cost-saving workflow is:

1. strong agent fixes the architecture;
2. main agent decomposes it;
3. focused agents implement bounded slices;
4. specialist reviewers inspect risky boundaries;
5. strong agent performs final integration review.
