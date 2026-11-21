import fs from 'node:fs';
import path from 'node:path';

export function readIndexNowKey(directory) {
  const files = fs.readdirSync(directory, { withFileTypes: true })
    .filter(entry => entry.isFile() && /^[a-f0-9]{32}\.txt$/.test(entry.name));
  if (files.length !== 1) {
    throw new Error('IndexNow requires exactly one public key file in ' + directory);
  }
  const { name } = files[0];
  const content = fs.readFileSync(path.join(directory, name));
  if (!content.equals(Buffer.from(name.slice(0, -4), 'ascii'))) {
    throw new Error('IndexNow key file content must match its filename without .txt');
  }
  return { name, content };
}
