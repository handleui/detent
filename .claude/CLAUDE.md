# Detent Project Guidelines

## Context7 MCP
Always use Context7 for library/API documentation, code generation, or configuration steps without being asked.

## Commands (Critical)
- **Build**: `bun run build` (Turborepo - NEVER use `go build` directly)
- **Run CLI**: `detent` (shell alias - NEVER use `./dist/dt`)
- **Lint/Fix**: `bun run lint` / `bun run fix`
- **Types**: `bun run check-types`
- **Go lint**: `cd apps/cli && golangci-lint run ./...`
- **Go test**: `cd apps/cli && go test ./...`

## Git
- Conventional commits, header only, no description

## Project Structure
- Turborepo monorepo: `apps/cli` (Go), `apps/web` (Next.js)
- Shared packages: `packages/*` (ui, typescript-config)
- Formatter: Biome via ultracite (`bun run fix` auto-fixes)

## Style Deviations from Defaults
- **Files**: kebab-case (`user-profile.tsx`)
- **Types**: Interfaces over type aliases; import with `type` keyword
- **Functions**: Arrow functions only, no `function` declarations
- **Type casting**: Avoid `as` unless absolutely necessary
- **Comments**: None unless critical; prefix hacks with `// HACK: reason`
- **Go errors**: Prefix with `Err`, handle explicitly
