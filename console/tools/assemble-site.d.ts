// Typings for the site assembler (assemble-site.mjs), so the Vitest suite
// can hold its route list against the chrome's Workspace list. This file
// types only the module boundary.

export const WORKSPACE_ROUTES: string[]
export function entryDocuments(routes?: string[]): string[]
