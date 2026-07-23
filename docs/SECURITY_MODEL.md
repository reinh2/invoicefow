# Security Model

## Assets

- original documents;
- extracted financial data;
- approval decisions;
- audit history;
- storage;
- database;
- provider and webhook secrets.

## Threats

- malicious file upload;
- path traversal or symlink escape;
- command injection;
- oversized files and process resource exhaustion;
- PDF/OCR hangs;
- prompt injection in document text;
- model output manipulating trusted state;
- unauthorized approval/export;
- forged or replayed webhooks;
- duplicate side effects;
- secrets or document text leaking through logs;
- real personal data committed to Git.

## Release rule

Stage 0 maps each threat to components, controls, and tests. The MVP must not claim PCI, GDPR, tax, legal, or accounting compliance.
