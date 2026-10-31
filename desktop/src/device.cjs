"use strict";
const crypto = require("node:crypto");
function hash(value) { return crypto.createHash("sha256").update(value).digest("hex"); }
function identity(privateKey) {
  const pub = crypto.createPublicKey(privateKey).export({ format: "der", type: "spki" }).subarray(-32);
  const field = value => { const size = Buffer.alloc(4); size.writeUInt32BE(value.length); return Buffer.concat([size, value]); };
  return { id: `device-${hash(pub).slice(0,16)}`, publicKey: `ssh-ed25519 ${Buffer.concat([field(Buffer.from("ssh-ed25519")), field(pub)]).toString("base64")} safe-devops` };
}
function signRequest(privateKey, method, requestPath, body, credential, timestamp = Math.floor(Date.now()/1000), nonce = crypto.randomBytes(16).toString("hex")) {
  const {id} = identity(privateKey);
  const message = "safeops.device-request.v1\n" + [id, String(timestamp), nonce, method, requestPath, hash(body), hash(credential)].join("\n");
  return {"X-Safe-Devops-Device":id,"X-Safe-Devops-Time":String(timestamp),"X-Safe-Devops-Nonce":nonce,"X-Safe-Devops-Signature":crypto.sign(null,Buffer.from(message),privateKey).toString("base64")};
}
async function requestBytes(parts, session) {
  const buffers=[]; let size=0;
  for (const part of parts || []) {
    // Never read renderer-supplied filesystem paths in the privileged process.
    if (part.file) throw new Error("File uploads are disabled");
    const bytes = part.blobUUID ? await session.getBlobData(part.blobUUID) : part.bytes;
    if (!bytes) throw new Error("Unsupported request body");
    size += bytes.length; if(size>32768) throw new Error("Request body too large"); buffers.push(Buffer.from(bytes));
  }
  return Buffer.concat(buffers);
}
module.exports={identity,signRequest,requestBytes};
