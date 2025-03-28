use std::ffi::c_void;

use namada_core::borsh::BorshDeserialize;
use namada_core::masp_primitives::transaction::Transaction;
use namada_ibc::{decode_message, extract_masp_tx_from_envelope, IbcMessage};
use namada_tx::{Data, Section, Tx};

extern "C" {
    fn append_masp_tx(
        map_handle: *mut c_void,
        masp_tx_id: *const [u8; 32],
        section_hash: *const [u8; 32],
        is_ibc: i8,
    );
}

#[no_mangle]
extern "C" fn locate_masp_tx_ids_in_nam_tx(
    result_ptr: *mut c_void,
    nam_tx_borsh_data_ptr: *const u8,
    nam_tx_borsh_data_len: usize,
) -> i8 {
    if result_ptr.is_null() || nam_tx_borsh_data_ptr.is_null() {
        return i8::MIN;
    }

    let nam_tx_borsh_data =
        unsafe { std::slice::from_raw_parts(nam_tx_borsh_data_ptr, nam_tx_borsh_data_len) };

    let Ok(tx) = Tx::try_from_slice(nam_tx_borsh_data) else {
        return -1;
    };

    for section in tx.sections.iter() {
        match section {
            Section::MaspTx(masp_tx) => {
                let txid = masp_tx.txid();

                unsafe {
                    append_masp_tx(result_ptr, txid.as_ref(), txid.as_ref(), 0i8);
                }
            }
            Section::Data(Data { data, .. }) => {
                let Ok(IbcMessage::Envelope(envelope)) =
                    decode_message::<namada_token::Transfer>(data)
                else {
                    continue;
                };

                let Some(masp_tx) = extract_masp_tx_from_envelope(&envelope) else {
                    continue;
                };

                let txid = masp_tx.txid();
                let sechash = section.get_hash().0;

                unsafe {
                    append_masp_tx(result_ptr, txid.as_ref(), &sechash, 1i8);
                }
            }
            _ => {}
        }
    }

    0
}

#[no_mangle]
extern "C" fn compute_masp_tx_id(
    result_ptr: *mut [u8; 32],
    borsh_data_ptr: *const u8,
    borsh_data_len: usize,
) -> i8 {
    if result_ptr.is_null() || borsh_data_ptr.is_null() {
        return i8::MIN;
    }

    let borsh_data = unsafe { std::slice::from_raw_parts(borsh_data_ptr, borsh_data_len) };

    let Ok(masp_tx) = Transaction::try_from_slice(borsh_data) else {
        return -1;
    };

    let id = masp_tx.txid();

    unsafe {
        result_ptr.write(*id.as_ref());
    }

    0
}
