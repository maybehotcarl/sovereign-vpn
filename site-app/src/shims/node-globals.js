import { Buffer } from 'buffer';

if (!globalThis.global) {
  globalThis.global = globalThis;
}

if (!globalThis.Buffer) {
  globalThis.Buffer = Buffer;
}

if (!globalThis.process) {
  globalThis.process = {
    env: {},
    browser: true,
  };
} else {
  globalThis.process.env ||= {};
  globalThis.process.browser ??= true;
}
