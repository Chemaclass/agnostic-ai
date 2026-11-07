# Interface fonts

The interface ships two font families inside the binary. It loads no network font.

| File | Family | Weight | Characters |
| --- | --- | --- | --- |
| `archivo-narrow-700.woff2` | Archivo Narrow | 700 | U+0020-007E, U+00B7, U+2013-2014, U+2019, U+2026 |
| `atkinson-next-400.woff2` | Atkinson Hyperlegible Next | 400 | U+0020-007E, U+00A0-00FF, U+0131, U+0152-0153, U+2013-2014, U+2018-2019, U+201C-201D, U+2022, U+2026, U+2190-2193, U+2212 |
| `atkinson-next-600.woff2` | Atkinson Hyperlegible Next | 600 | same as 400 |

Both families use the SIL Open Font License 1.1: see `OFL-ArchivoNarrow.txt` and `OFL-AtkinsonHyperlegibleNext.txt`. Neither copyright notice declares a Reserved Font Name, so the subsets keep their family names. `npm run bundle` copies both license texts into `third-party-notices.txt`.

## Source

[google/fonts](https://github.com/google/fonts) at commit `86aced91de05cd4f96a06838124cca45be337b2b`:

| Source | sha256 |
| --- | --- |
| `ofl/archivonarrow/ArchivoNarrow[wght].ttf` | `adbe027f625c8393ae0f6e174e32e233dda485bc3eda5153ce428275394ef97f` |
| `ofl/atkinsonhyperlegiblenext/AtkinsonHyperlegibleNext[wght].ttf` | `5a455d1cfa099b601ab70751bb9673e8fe1854dc4500c80e1a220d0d75e31745` |

## Rebuild

The steps use fontTools 4.60.2 and Brotli 1.2.1 in a virtual environment. Keep the downloads in their own empty folder and run Python with `-I`, so it loads no module from the current folder. `SOURCE_DATE_EPOCH=0` fixes the timestamp inside each font, so the output matches byte for byte.

```sh
fonts="$PWD/ui/src/assets/fonts"
work="$(mktemp -d)"
mkdir "$work/src" "$work/build"
commit=86aced91de05cd4f96a06838124cca45be337b2b
base="https://raw.githubusercontent.com/google/fonts/$commit/ofl"
curl -sfL -o "$work/src/ArchivoNarrow-wght.ttf" "$base/archivonarrow/ArchivoNarrow%5Bwght%5D.ttf"
curl -sfL -o "$work/src/AtkinsonNext-wght.ttf" "$base/atkinsonhyperlegiblenext/AtkinsonHyperlegibleNext%5Bwght%5D.ttf"
curl -sfL -o "$fonts/OFL-ArchivoNarrow.txt" "$base/archivonarrow/OFL.txt"
curl -sfL -o "$fonts/OFL-AtkinsonHyperlegibleNext.txt" "$base/atkinsonhyperlegiblenext/OFL.txt"
shasum -a 256 "$work"/src/*.ttf

python3 -m venv "$work/venv"
"$work/venv/bin/python" -m pip install fonttools==4.60.2 brotli==1.2.1
py="$work/venv/bin/python -I"
cd "$work"
export SOURCE_DATE_EPOCH=0

$py -m fontTools.varLib.instancer -q src/ArchivoNarrow-wght.ttf wght=700 -o build/archivo-narrow-700.ttf
$py -m fontTools.varLib.instancer -q src/AtkinsonNext-wght.ttf wght=400 -o build/atkinson-next-400.ttf
$py -m fontTools.varLib.instancer -q src/AtkinsonNext-wght.ttf wght=600 -o build/atkinson-next-600.ttf

keep="--layout-features+=tnum,case --name-IDs=0,1,2,3,4,5,6,13,14 --flavor=woff2"
$py -m fontTools.subset build/archivo-narrow-700.ttf $keep \
  --unicodes=U+0020-007E,U+00B7,U+2013-2014,U+2019,U+2026 \
  --output-file="$fonts/archivo-narrow-700.woff2"
for weight in 400 600; do
  $py -m fontTools.subset "build/atkinson-next-$weight.ttf" $keep \
    --unicodes=U+0020-007E,U+00A0-00FF,U+0131,U+0152-0153,U+2013-2014,U+2018-2019,U+201C-201D,U+2022,U+2026,U+2190-2193,U+2212 \
    --output-file="$fonts/atkinson-next-$weight.woff2"
done
shasum -a 256 "$fonts"/*.woff2
```

Run it from the repository root in `sh` or `bash`; zsh does not split `$py` and `$keep` into words. The output should match:

| File | Bytes | sha256 |
| --- | --- | --- |
| `archivo-narrow-700.woff2` | 6936 | `40f78faef8b40cb132141740fb3487dd951e07ace28d1494b1aa0b038a240414` |
| `atkinson-next-400.woff2` | 10648 | `d64da53e154162dcd2d984818fb8036c207f8ee52f63d8c44ce508b5f5a1027c` |
| `atkinson-next-600.woff2` | 11176 | `7e611aee93618cb770839dd0d85ffcffa0ad6d01acd1dc18a46dfbd2900b341b` |

## Fallback faces

`src/styles/tokens.css` declares a fallback face for each font on the local Arial, scaled so text keeps its width while the real font loads. The numbers compare the average advance of `The quick brown fox jumps over the lazy dog 0123456789` with Arial (Arial Bold for 600 and 700); ascent and descent come from the font's typographic metrics divided by the size adjustment.

| Fallback | Local font | size-adjust | ascent-override | descent-override |
| --- | --- | --- | --- | --- |
| Atkinson 400 | Arial | 99.86% | 98.54% | 31.64% |
| Atkinson 600 | Arial Bold | 98.39% | 100.01% | 32.12% |
| Archivo Narrow 700 | Arial Bold | 82.01% | 126.20% | 38.04% |
