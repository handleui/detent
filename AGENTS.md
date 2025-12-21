# Agent Development Guidelines

## Commands
- Build: `bun run build` | Test: (none configured) | Lint: `bun run lint` | Fix: `bun run fix`
- Type check: `bun run check-types` | Dev: `bun run dev`
- Go lint: `cd apps/cli && golangci-lint run ./...` | Go fix: `cd apps/cli && golangci-lint run --fix ./...`

## TypeScript Style
- **Files**: kebab-case (e.g., `user-profile.tsx`)
- **Types**: Use interfaces over types. Keep all types in global scope. Import types with `type` keyword: `import type { Foo } from "bar"`
- **Functions**: Arrow functions only. No function declarations
- **Naming**: Frequently re-evaluate variable names for accuracy and clarity
- **Type safety**: Never use "as" type casting unless absolutely necessary
- **Clean code**: Remove unused code, no repetition, no comments unless absolutely necessary
- **Hacks**: Prefix with `// HACK: reason` for workarounds (setTimeout, confusing code)
- **React**: Function components, hooks at top level, proper dependencies, semantic HTML with ARIA
- **Next.js**: Use `<Image>` component, Server Components for async data, App Router metadata API
- **Async**: Always await promises, use async/await over chains, handle errors with try-catch

## Go Style
- Follow golangci-lint standard preset with gosec, gocritic, misspell, errname, exhaustive enabled
- Handle errors explicitly, use clear naming conventions (errors prefixed with Err)
- Enable shadow and nilness checks via govet

## Project Structure
- Turborepo monorepo with apps/cli (Go) and apps/web (Next.js)
- Shared packages in packages/* (ui, typescript-config)
- Formatter: Biome via ultracite (extends ultracite/core and ultracite/next)
