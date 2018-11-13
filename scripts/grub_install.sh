#!/bin/bash

set -e

disk_image="${1}"
LOOP_DEV="${2}"
ESP_DIR="${3}"

# i386-pc or x86_64-efi
target=${4:-x86_64-efi}

export PATH="/opt/grub/bin:/opt/grub/sbin:$PATH"

info() {
    echo "INFO:" "$@"
}

warn() {
    echo "WARN:" "$@"
}

cleanup() {
    if [[ -n "${GRUB_TEMP_DIR}" && -e "${GRUB_TEMP_DIR}" ]]; then
      rm -r "${GRUB_TEMP_DIR}"
    fi
}
trap cleanup EXIT

info "Installing GRUB in ${disk_image##*/}"

mkdir -p "${ESP_DIR}/grub"
GRUB_TEMP_DIR="$(mktemp -d)"

devmap="${GRUB_TEMP_DIR}/device.map"

cat >${devmap} <<EOF
(hd0)   ${LOOP_DEV}
EOF

mkdir -p "${ESP_DIR}/grub"
sed -e "s/{{.FSID}}/$ESP_FSID/g" "/assets/grub.cfg" > "${ESP_DIR}/grub/grub.cfg"

info "Generating grub/load.cfg"
# Include a small initial config in the core image to search for the ESP
# by filesystem ID in case the platform doesn't provide the boot disk.
# The existing $root value is given as a hint so it is searched first.
ESP_FSID=$(grub-probe --device-map=$devmap -t fs_uuid -d "${LOOP_DEV}p1")
cat > "${ESP_DIR}/grub/load.cfg" <<EOF
search.fs_uuid ${ESP_FSID} root \$root
set prefix=(memdisk)
set
EOF

# Generate a memdisk containing the appropriately generated grub.cfg. Doing
# this because we need conflicting default behaviors between verity and
# non-verity images.
info "Generating grub.cfg memdisk"

sed -e "s/{{.FSID}}/$ESP_FSID/g" "/assets/grub.cfg" > "${GRUB_TEMP_DIR}/grub.cfg"

mkdir -p "${ESP_DIR}/grub"
tar cf "${ESP_DIR}/grub/grub.cfg.tar" \
 -C "${GRUB_TEMP_DIR}" "grub.cfg"

for target in i386-pc x86_64-efi
do
    GRUB_SRC="/usr/lib/grub/${target}"
    GRUB_DIR="grub/${target}"

    [[ -d "${GRUB_SRC}" ]] || die "GRUB not installed at ${GRUB_SRC}"

    info "Compressing modules in ${GRUB_DIR}"
    mkdir -p "${ESP_DIR}/${GRUB_DIR}"
    for file in "${GRUB_SRC}"/*{.lst,.mod}; do
        out="${ESP_DIR}/${GRUB_DIR}/${file##*/}"
        gzip --best --stdout "${file}" | cat > "${out}"
    done

    # Modules required to boot a standard configuration
    CORE_MODULES=( normal search test fat part_gpt search_fs_uuid gzio terminal configfile memdisk tar echo read )
    #CORE_MODULES+=( search_part_label gptprio )

    # Name of the core image, depends on target
    CORE_NAME=

    case "${target}" in
        i386-pc)
            CORE_MODULES+=( biosdisk serial linux )
            CORE_NAME="core.img"
            ;;
        x86_64-efi)
            CORE_MODULES+=( serial linuxefi efi_gop efinet verify http tftp )
            #CORE_MODULES+=( getenv smbios )
            CORE_NAME="core.efi"
            ;;
        *)
            die_notrace "Unknown GRUB target ${target}"
            ;;
    esac

    info "Generating grub/${CORE_NAME}"
    grub-mkimage \
        --compression=auto \
        --format "${target}" \
        --directory "${GRUB_SRC}" \
        --config "${ESP_DIR}/grub/load.cfg" \
        --memdisk "${ESP_DIR}/grub/grub.cfg.tar" \
        --output "${ESP_DIR}/${GRUB_DIR}/${CORE_NAME}" \
        "${CORE_MODULES[@]}"
done

# Now target specific steps to make the system bootable
info "Installing MBR and the BIOS Boot partition."
cp "/usr/lib/grub/i386-pc/boot.img" "${ESP_DIR}/grub/i386-pc"
grub-bios-setup --device-map=$devmap \
    --directory="${ESP_DIR}/grub/i386-pc" "${LOOP_DEV}"

# backup the MBR
dd bs=448 count=1 status=none if="${LOOP_DEV}" \
    of="${ESP_DIR}/grub/mbr.bin"

info "Installing default x86_64 UEFI bootloader."
mkdir -p "${ESP_DIR}/EFI/boot"

cp "${ESP_DIR}/grub/x86_64-efi/core.efi" \
    "${ESP_DIR}/EFI/boot/grubx64.efi"
cp "/shim/BOOTX64.EFI" \
    "${ESP_DIR}/EFI/boot/bootx64.efi"

cleanup
trap - EXIT
