package collect

// LocationConsentDenied reads, read-only, the Windows Location consent value (machine and user).
// It gives denied true only when a value says "Deny"; known is false when nothing could be read.
// It is a variable so tests can replace it. Other systems have no such value (ND-242).
var LocationConsentDenied = readLocationConsent
