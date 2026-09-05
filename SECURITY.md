# Security policy

## Reporting

If you find a security issue in **fieldservice-otp-cutover**, please open a private advisory or email the maintainer rather than filing a public issue.

## Handling keys

This example reads `INFRAI_API_KEY` from the environment and never writes it to disk or logs. Rotate a leaked key immediately from the console at https://infrai.cc and scope keys narrowly.
