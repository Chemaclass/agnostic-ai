import { createHash } from 'node:crypto'
import { mkdtemp, readFile, readdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const check = process.argv.includes('--check')
const temporary = check ? await mkdtemp(join(tmpdir(), 'agnostic-ui-bundle-')) : null
const output = temporary ?? join(root, 'embedded')

async function files(directory) {
  const entries = await readdir(directory, { withFileTypes: true })
  const paths = await Promise.all(entries.map(async (entry) => {
    const path = join(directory, entry.name)
    if (entry.isSymbolicLink()) throw new Error(`Bundle inputs cannot be symlinks: ${path}`)
    return entry.isDirectory() ? files(path) : [path]
  }))
  return paths.flat().sort()
}

async function hashes(paths, base) {
  const result = {}
  for (const path of paths) {
    result[relative(base, path).replaceAll('\\', '/')] = createHash('sha256').update(await readFile(path)).digest('hex')
  }
  return result
}

try {
  await build({
    root,
    configFile: join(root, 'vite.config.ts'),
    envDir: false,
    base: '/',
    define: { 'import.meta.env.VITE_DEFAULT_SOURCE': 'undefined' },
    build: { outDir: output, emptyOutDir: true },
  })
  const notices = []
  for (const name of ['react', 'react-dom', 'scheduler']) {
    const directory = join(root, 'node_modules', name)
    const metadata = JSON.parse(await readFile(join(directory, 'package.json'), 'utf8'))
    notices.push(`${name} ${metadata.version}\n\n${await readFile(join(directory, 'LICENSE'), 'utf8')}`)
  }
  notices.push(`Vite runtime helpers\n\n${await readFile(join(root, 'node_modules/vite/LICENSE.md'), 'utf8')}`)
  notices.push(`${await readFile(join(root, 'src/assets/tools/README.md'), 'utf8')}\n${await readFile(join(root, 'src/assets/tools/LICENSE'), 'utf8')}`)
  await writeFile(join(output, 'third-party-notices.txt'), notices.join('\n\n'))
  const inputs = [
    ...['index.html', 'package.json', 'package-lock.json', 'tsconfig.json', 'vite.config.ts'].map((file) => join(root, file)),
    ...await files(join(root, 'src')),
    ...await files(join(root, 'scripts')),
  ].sort()
  const manifest = {
    version: 1,
    inputs: await hashes(inputs, root),
    files: await hashes(await files(output), output),
  }
  await writeFile(join(output, '.manifest.json'), `${JSON.stringify(manifest, null, 2)}\n`)
  if (check) {
    const expected = await hashes(await files(output), output)
    const actual = await hashes(await files(join(root, 'embedded')), join(root, 'embedded'))
    if (JSON.stringify(expected) !== JSON.stringify(actual)) {
      throw new Error('Bundled UI is stale. Run npm run bundle in ui/ and commit ui/embedded/.')
    }
    console.log('Bundled UI matches its sources and lockfile.')
  }
} finally {
  if (temporary) await rm(temporary, { recursive: true, force: true })
}
