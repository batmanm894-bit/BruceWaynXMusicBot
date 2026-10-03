// Created by Laky64 on 23/04/2025.
// This header file provides compatibility for glibc >= 2.34 (libresolv was
// merged into libc): static libs built against older glibc still reference the
// legacy symbol names __dn_expand and __res_nquery, which newer glibc no longer
// exports as default symbols. These wrappers forward to the real functions.
//
// NOTE: the previous version of __dn_expand was NOT equivalent to the real
// function (different argument meaning; it called res_query() and could write
// network data into the caller's buffers). This one forwards 1:1.

#pragma once

#ifdef __GLIBC__

#if __GLIBC__ > 2 || (__GLIBC__ == 2 && __GLIBC_MINOR__ >= 28)

#include <resolv.h>

// Bind to the real libc symbols by assembler name so that no <resolv.h> macro
// renaming can turn these into self-recursive calls.
extern int ntg_real_dn_expand(const unsigned char *, const unsigned char *,
                              const unsigned char *, char *, int)
    __asm__("dn_expand");
extern int ntg_real_res_nquery(res_state, const char *, int, int,
                               unsigned char *, int) __asm__("res_nquery");

int __dn_expand(
    const unsigned char *msg,
    const unsigned char *eom,
    const unsigned char *src,
    char *dst,
    int dstsiz
) {
    return ntg_real_dn_expand(msg, eom, src, dst, dstsiz);
}

int __res_nquery(
    res_state statp,
    const char *dname,
    int class,
    int type,
    unsigned char *answer,
    int anslen
) {
    return ntg_real_res_nquery(statp, dname, class, type, answer, anslen);
}

#endif
#endif
