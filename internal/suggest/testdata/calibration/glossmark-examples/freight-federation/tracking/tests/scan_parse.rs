// @gloss verifies .ScanIngestor scope=file conf=0.9
use kestrel_tracking::ingest::scan_ingestor::{luhn_check_digit, parse_tracking_number, ScanParseError};

#[test]
fn luhn_check_digit_of_reference_number() {
    assert_eq!(luhn_check_digit("1234567890"), Some(3));
    assert_eq!(luhn_check_digit("12345X7890"), None);
}

#[test]
fn parses_plain_tracking_number() {
    let tn = parse_tracking_number("KF12345678903").unwrap();
    assert_eq!(tn.as_str(), "KF12345678903");
}

#[test]
fn strips_code128_symbology_identifier_and_whitespace() {
    let tn = parse_tracking_number("]C1KF12345678903\r\n").unwrap();
    assert_eq!(tn.as_str(), "KF12345678903");
}

#[test]
fn strips_fnc1_group_separator() {
    let tn = parse_tracking_number("\u{1d}KF12345678903").unwrap();
    assert_eq!(tn.as_str(), "KF12345678903");
}

#[test]
fn normalises_lowercase_prefix() {
    let tn = parse_tracking_number("kf12345678903").unwrap();
    assert_eq!(tn.to_string(), "KF12345678903");
}

#[test]
fn rejects_tampered_check_digit() {
    assert_eq!(
        parse_tracking_number("KF12345678904"),
        Err(ScanParseError::CheckDigitMismatch { expected: 3, found: 4 })
    );
}

#[test]
fn rejects_transposed_digits() {
    assert!(matches!(
        parse_tracking_number("KF21345678903"),
        Err(ScanParseError::CheckDigitMismatch { .. })
    ));
}

#[test]
fn rejects_wrong_prefix() {
    assert_eq!(parse_tracking_number("KX12345678903"), Err(ScanParseError::WrongPrefix));
}

#[test]
fn rejects_wrong_length() {
    assert_eq!(parse_tracking_number("KF123"), Err(ScanParseError::WrongLength(5)));
}

#[test]
fn rejects_letters_in_digit_section() {
    assert_eq!(parse_tracking_number("KF1234567890A"), Err(ScanParseError::NonDigit));
}

#[test]
fn rejects_empty_scan() {
    assert_eq!(parse_tracking_number("   "), Err(ScanParseError::Empty));
}
