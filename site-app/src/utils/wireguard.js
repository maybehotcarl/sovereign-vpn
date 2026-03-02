// WireGuard key generation — pure JS Curve25519 (x25519)
// Generates a clamped private key and derives the public key via scalar base multiplication.

function gf(init) {
  const r = new Float64Array(16);
  if (init) for (let i = 0; i < init.length; i++) r[i] = init[i];
  return r;
}

const _121665 = gf([0xdb41, 1]);

function car25519(o) {
  let c;
  for (let i = 0; i < 16; i++) {
    o[i] += 65536;
    c = Math.floor(o[i] / 65536);
    o[(i + 1) % 16] += c - 1 + 37 * (c - 1) * (i === 15 ? 1 : 0);
    o[i] -= c * 65536;
  }
}

function sel25519(p, q, b) {
  const c = ~(b - 1);
  for (let i = 0; i < 16; i++) {
    const t = c & (p[i] ^ q[i]);
    p[i] ^= t;
    q[i] ^= t;
  }
}

function A(o, a, b) { for (let i = 0; i < 16; i++) o[i] = a[i] + b[i]; }
function Z(o, a, b) { for (let i = 0; i < 16; i++) o[i] = a[i] - b[i]; }

function M(o, a, b) {
  const t = new Float64Array(31);
  for (let i = 0; i < 16; i++)
    for (let j = 0; j < 16; j++) t[i + j] += a[i] * b[j];
  for (let i = 0; i < 15; i++) t[i] += 38 * t[i + 16];
  for (let i = 0; i < 16; i++) o[i] = t[i];
  car25519(o);
  car25519(o);
}

function S(o, a) { M(o, a, a); }

function inv25519(o, a) {
  const c = gf();
  for (let i = 0; i < 16; i++) c[i] = a[i];
  for (let i = 253; i >= 0; i--) {
    S(c, c);
    if (i !== 2 && i !== 4) M(c, c, a);
  }
  for (let i = 0; i < 16; i++) o[i] = c[i];
}

function pack25519(o, n) {
  const m = gf(), t = gf();
  for (let i = 0; i < 16; i++) t[i] = n[i];
  car25519(t); car25519(t); car25519(t);
  for (let j = 0; j < 2; j++) {
    m[0] = t[0] - 0xffed;
    for (let i = 1; i < 15; i++) {
      m[i] = t[i] - 0xffff - ((m[i - 1] >> 16) & 1);
      m[i - 1] &= 0xffff;
    }
    m[15] = t[15] - 0x7fff - ((m[14] >> 16) & 1);
    const b = (m[15] >> 16) & 1;
    m[14] &= 0xffff;
    sel25519(t, m, 1 - b);
  }
  for (let i = 0; i < 16; i++) {
    o[2 * i] = t[i] & 0xff;
    o[2 * i + 1] = t[i] >> 8;
  }
}

function scalarbase(q, n) {
  const _9 = gf([9]);
  const a = gf(), b = gf([9]), c = gf(), d = gf([1]), e = gf(), f = gf();
  a[0] = 1;
  for (let i = 254; i >= 0; i--) {
    const r = (n[i >>> 3] >>> (i & 7)) & 1;
    sel25519(a, b, r); sel25519(c, d, r);
    A(e, a, c); Z(a, a, c); A(c, b, d); Z(b, b, d);
    S(d, e); S(f, a); M(a, c, a); M(c, b, e);
    A(e, a, c); Z(a, a, c); S(b, a); Z(c, d, f);
    M(a, c, _121665); A(a, a, d); M(c, c, a); M(a, d, f); M(d, b, _9); S(b, e);
    sel25519(a, b, r); sel25519(c, d, r);
  }
  const t = gf();
  inv25519(t, c);
  M(a, a, t);
  pack25519(q, a);
}

export function generateKeyPair() {
  const privateKey = new Uint8Array(32);
  crypto.getRandomValues(privateKey);
  // Clamp per Curve25519 spec
  privateKey[0] &= 248;
  privateKey[31] &= 127;
  privateKey[31] |= 64;

  const publicKey = new Uint8Array(32);
  scalarbase(publicKey, privateKey);

  return {
    privateKey: btoa(String.fromCharCode(...privateKey)),
    publicKey: btoa(String.fromCharCode(...publicKey)),
  };
}
