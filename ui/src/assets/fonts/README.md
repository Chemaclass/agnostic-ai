# Interface fonts

Both font families ship in the binary and load without network requests.

| File | Family | Weight | Characters |
| --- | --- | --- | --- |
| `atkinson-next-400.woff2` | Atkinson Hyperlegible Next | 400 | U+0020-007E, U+00A0-00FF, U+0131, U+0152-0153, U+2013-2014, U+2018-2019, U+201C-201D, U+2022, U+2026, U+2190-2193, U+2212 |
| `atkinson-next-600.woff2` | Atkinson Hyperlegible Next | 600 | same as 400 |
| `fraunces-500.woff2` | Fraunces | 500 | same as 400 |
| `fraunces-600.woff2` | Fraunces | 600 | same as 400 |

Fraunces is a variable font with optical size and two style axes; the subsets fix it at optical size 72, `SOFT` 0 and `WONK` 0, roman only.

Both families use the SIL Open Font License 1.1: see `OFL-AtkinsonHyperlegibleNext.txt` and `OFL-Fraunces.txt`. Neither copyright notice declares a Reserved Font Name, so the subsets keep their family names. `npm run bundle` copies both license texts into `third-party-notices.txt`.

## Source

[google/fonts](https://github.com/google/fonts) at commit `86aced91de05cd4f96a06838124cca45be337b2b`:

| Source | sha256 |
| --- | --- |
| `ofl/atkinsonhyperlegiblenext/AtkinsonHyperlegibleNext[wght].ttf` | `5a455d1cfa099b601ab70751bb9673e8fe1854dc4500c80e1a220d0d75e31745` |
| `ofl/fraunces/Fraunces[SOFT,WONK,opsz,wght].ttf` | `177ff6c0f14e5550a3c624247cd1189611d4eb65d000b14944c63d967958abbb` |

## Rebuild

Use fontTools 4.60.2 and Brotli 1.2.1 in a virtual environment. Download into an empty folder and run Python with `-I` to prevent imports from that folder. `SOURCE_DATE_EPOCH=0` fixes font timestamps for reproducible output.

```sh
fonts="$PWD/ui/src/assets/fonts"
work="$(mktemp -d)"
mkdir "$work/src" "$work/build"
commit=86aced91de05cd4f96a06838124cca45be337b2b
base="https://raw.githubusercontent.com/google/fonts/$commit/ofl"
curl -sfL -o "$work/src/AtkinsonNext-wght.ttf" "$base/atkinsonhyperlegiblenext/AtkinsonHyperlegibleNext%5Bwght%5D.ttf"
curl -sfL -o "$work/src/Fraunces-wght.ttf" "$base/fraunces/Fraunces%5BSOFT,WONK,opsz,wght%5D.ttf"
curl -sfL -o "$fonts/OFL-AtkinsonHyperlegibleNext.txt" "$base/atkinsonhyperlegiblenext/OFL.txt"
curl -sfL -o "$fonts/OFL-Fraunces.txt" "$base/fraunces/OFL.txt"
shasum -a 256 "$work"/src/*.ttf

python3 -m venv "$work/venv"
"$work/venv/bin/python" -m pip install fonttools==4.60.2 brotli==1.2.1
py="$work/venv/bin/python -I"
cd "$work"
export SOURCE_DATE_EPOCH=0

keep="--layout-features+=tnum,case --name-IDs=0,1,2,3,4,5,6,13,14 --flavor=woff2"
chars="U+0020-007E,U+00A0-00FF,U+0131,U+0152-0153,U+2013-2014,U+2018-2019,U+201C-201D,U+2022,U+2026,U+2190-2193,U+2212"
for weight in 400 600; do
  $py -m fontTools.varLib.instancer -q src/AtkinsonNext-wght.ttf wght=$weight -o "build/atkinson-next-$weight.ttf"
  $py -m fontTools.subset "build/atkinson-next-$weight.ttf" $keep --unicodes="$chars" --output-file="$fonts/atkinson-next-$weight.woff2"
done
for weight in 500 600; do
  $py -m fontTools.varLib.instancer -q src/Fraunces-wght.ttf wght=$weight opsz=72 SOFT=0 WONK=0 -o "build/fraunces-$weight.ttf"
  $py -m fontTools.subset "build/fraunces-$weight.ttf" $keep --unicodes="$chars" --output-file="$fonts/fraunces-$weight.woff2"
done
shasum -a 256 "$fonts"/*.woff2
```

Run it from the repository root in `sh` or `bash`; zsh does not split `$py` and `$keep` into words. The output should match:

| File | Bytes | sha256 |
| --- | --- | --- |
| `atkinson-next-400.woff2` | 10648 | `d64da53e154162dcd2d984818fb8036c207f8ee52f63d8c44ce508b5f5a1027c` |
| `atkinson-next-600.woff2` | 11176 | `7e611aee93618cb770839dd0d85ffcffa0ad6d01acd1dc18a46dfbd2900b341b` |
| `fraunces-500.woff2` | 17296 | `c4b121e21d4498429d652c5596eab6ee60b4a1b7f81690b3c9735b5a35d08e9a` |
| `fraunces-600.woff2` | 17356 | `ca2d49b857fdc9d672315404239ec6442f487f98d742ad73cb89922aa5f802b7` |

## Fallback faces

`src/styles/tokens.css` scales Arial as the Atkinson Hyperlegible Next fallback and Georgia as the Fraunces fallback to preserve text width during loading. Size adjustments compare the average advance of `The quick brown fox jumps over the lazy dog 0123456789` in each face and its local fallback (bold for weight 600). Ascent and descent are the font's typographic metrics divided by the size adjustment.

| Fallback | Local font | size-adjust | ascent-override | descent-override |
| --- | --- | --- | --- | --- |
| Atkinson 400 | Arial | 99.86% | 98.54% | 31.64% |
| Atkinson 600 | Arial Bold | 98.39% | 100.01% | 32.12% |
| Fraunces 500 | Georgia | 98.65% | 99.14% | 25.85% |
| Fraunces 600 | Georgia Bold | 86.99% | 112.42% | 29.31% |
