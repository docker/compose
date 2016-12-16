#include <stddef.h>
#include <linux/magic.h>
#include <btrfs/ioctl.h>
#include <btrfs/ctree.h>

#include "btrfs.h"

void unpack_root_item(struct gosafe_btrfs_root_item* dst, struct btrfs_root_item* src) {
	memcpy(dst->uuid, src->uuid, BTRFS_UUID_SIZE);
	memcpy(dst->parent_uuid, src->parent_uuid, BTRFS_UUID_SIZE);
	memcpy(dst->received_uuid, src->received_uuid, BTRFS_UUID_SIZE);
	dst->gen = btrfs_root_generation(src);
	dst->ogen = btrfs_root_otransid(src);
	dst->flags = btrfs_root_flags(src);
}

/* unpack_root_ref(struct gosafe_btrfs_root_ref* dst, struct btrfs_root_ref* src) { */
