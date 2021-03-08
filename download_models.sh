#!/bin/sh
# https://github.com/facebookresearch/wav2letter/blob/master/recipes/rasr/README.md

DEST_DIR=$@
if [ -z "$DEST_DIR" ]; then
    echo "Usage: $0 DEST_DIR"
    exit 1
fi

get () {
    NAME=$(basename "$1")
    wget --no-clobber -O "$DEST_DIR/$NAME" "$1"
}

get https://dl.fbaipublicfiles.com/wav2letter/rasr/tutorial/am_transformer_ctc_stride3_letters_300Mparams.bin
get https://dl.fbaipublicfiles.com/wav2letter/rasr/tutorial/lm_common_crawl_large_4gram_prun0-0-5_200kvocab.bin
get https://dl.fbaipublicfiles.com/wav2letter/rasr/tutorial/lexicon.txt
get https://dl.fbaipublicfiles.com/wav2letter/rasr/tutorial/tokens.txt