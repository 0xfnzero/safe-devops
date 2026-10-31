const {test}=require("node:test");const assert=require("node:assert/strict");const crypto=require("node:crypto");const {identity,signRequest,requestBytes}=require("./device.cjs");
test("signatures bind identity, method, path, bytes, time and nonce",()=>{
 const {privateKey,publicKey}=crypto.generateKeyPairSync("ed25519");const id=identity(privateKey);assert.match(id.publicKey,/^ssh-ed25519 /);
 const body=Buffer.from('{"command":"ls"}');const headers=signRequest(privateKey,"POST","/api/terminal/command",body,"credential",1700000000,"0123456789abcdef0123456789abcdef");
 const hash=b=>crypto.createHash("sha256").update(b).digest("hex");
 const fields=[id.id,"1700000000","0123456789abcdef0123456789abcdef","POST","/api/terminal/command",hash(body),hash("credential")];
 const signature=Buffer.from(headers["X-Safe-Devops-Signature"],"base64");
 assert.equal(crypto.verify(null,Buffer.from("safeops.device-request.v1\n"+fields.join("\n")),publicKey,signature),true);
 for(let n=0;n<fields.length;n++){const changed=[...fields];changed[n]+="x";assert.equal(crypto.verify(null,Buffer.from("safeops.device-request.v1\n"+changed.join("\n")),publicKey,signature),false);}
 assert.notEqual(signRequest(privateKey,"GET","/api/me",Buffer.alloc(0),"credential")["X-Safe-Devops-Nonce"],signRequest(privateKey,"GET","/api/me",Buffer.alloc(0),"credential")["X-Safe-Devops-Nonce"]);
});
test("request signing never reads renderer filesystem paths and bounds blobs",async()=>{
 await assert.rejects(()=>requestBytes([{file:"/etc/passwd"}],{}));
 assert.equal((await requestBytes([{bytes:Buffer.from("a")},{blobUUID:"blob"}],{getBlobData:async()=>Buffer.from("b")})).toString(),"ab");
 await assert.rejects(()=>requestBytes([{bytes:Buffer.alloc(32769)}],{}));
});
