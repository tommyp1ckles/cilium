// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright Authors of Cilium */

#include <bpf/ctx/unspec.h>
#include <bpf/api.h>

#include <node_config.h>
#include <lib/static_data.h>

#include "bpf/compiler.h"
#include "lib/common.h"
#include "lib/sock.h"
#include "lib/dbg.h"
#include "lib/sock_term.h"

struct sock_term_filter cilium_sock_term_filter;

/* Stub out types that would normally be found in vmlinux.h to satisfy BTF type
 * checks
 */
struct seq_file {};

struct bpf_iter_meta {
	struct seq_file *seq;
};

struct bpf_iter__udp {
	struct bpf_iter_meta *meta;
	void *udp_sk;
};

struct bpf_iter__tcp {
	struct bpf_iter_meta *meta;
	void *tcp_sk;
};

struct sock_common {};

#ifndef BPF_TEST
int bpf_sock_destroy(struct sock_common *sk) __section(".ksyms");
static int BPF_FUNC(seq_write, struct seq_file *m, const void *data,
		    __u32 len);
#endif

__section("iter/tcp")
int cil_sock_tcp_destroy_v4(struct bpf_iter__tcp *_)
{
	printk("yay");
	return 0;
}

BPF_LICENSE("Dual BSD/GPL");
