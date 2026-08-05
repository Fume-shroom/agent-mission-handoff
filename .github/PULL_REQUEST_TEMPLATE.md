## Summary

Describe the mission-handoff problem and the implemented behavior.

## Validation

- [ ] `go test ./...`
- [ ] `go test -race ./...`
- [ ] `go vet ./...`
- [ ] Sender behavior tested
- [ ] Receiver behavior tested
- [ ] Documentation updated when public behavior changed

## Safety

- [ ] Imported capsule data is still treated as untrusted
- [ ] Auth stores and permission state remain target-local; sensitive Session content is handled explicitly
- [ ] Real Session fixtures and logs were sanitized before committing
- [ ] Third-party attribution remains intact

## UX

- [ ] The normal workflow remains `amh pack` and `amh continue FILE`
- [ ] Any additional user interaction is limited to ambiguity, capability gaps, or approval requirements
