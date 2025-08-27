// SPDX-License-Identifier: (GPL-2.0-only OR BSD-2-Clause)
/* Copyright Authors of Cilium */
#include <linux/bpf.h>

//#include <linux/types.h>
//#include <linux/bpf_common.h>

#include <bpf/ctx/unspec.h>
#include <bpf/api.h>
#include <bpf/ctx/skb.h>

#include <node_config.h>
#include <lib/static_data.h>

#include "bpf/compiler.h"
#include "lib/common.h"
#include "lib/sock.h"
#include "lib/sock_term.h"
#include "lib/dbg.h"
#include "lib/conntrack.h"

struct bpf_map {
        __u32 id;
        char name[16];
        __u32 max_entries;
};

struct bpf_iter__bpf_map {
        struct bpf_iter_meta *meta;
        struct bpf_map *map;
};

struct bpf_iter_meta {
        __bpf_md_ptr(struct seq_file *, seq);
        __u64 session_id;
        __u64 seq_num;
};

struct bpf_iter__bpf_map_elem {
        __bpf_md_ptr(struct bpf_iter_meta *, meta);
        __bpf_md_ptr(struct bpf_map *, map);
        __bpf_md_ptr(void *, key);
        __bpf_md_ptr(void *, value);
};

//int iterate_ct(struct bpf_iter__bpf_map *ctx)
__section("iter/bpf_map_elem")
int iterate_ct(struct bpf_iter__bpf_map_elem *ctx)
{
	struct ipv4_ct_tuple *key = (struct ipv4_ct_tuple*) ctx->key;
	//struct ct_entry *value = (struct ct_entry*) ctx->value;
	//ctx->
	//printk2("elem -> %d", key->dport);
	printk2("elem -> ");
	printk2("iter -> %d!", key->sport);
	return 0;
}

BPF_LICENSE("Dual BSD/GPL");
