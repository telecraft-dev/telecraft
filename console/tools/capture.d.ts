// Typings for the capture tool (capture.mjs), so playwright.config.ts and
// the Vitest suite read the one output directory every capture goes to.
// This file types only the module boundary.

export const OUTPUT_DIR: string
export function capturePath(name: string): string
