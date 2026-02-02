import React, { useState } from "react";

// Minimal drop-in uploader for Bunny via your backend /api/v1/events/:id/image
// Requires a valid admin JWT token and an existing eventId.

type Props = {
  eventId: string;
  token: string;
  apiBase?: string; // defaults to localhost API if not provided
  onUploaded?: (url: string) => void;
};

export default function TestUpload({ eventId, token, apiBase, onUploaded }: Props) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [url, setUrl] = useState<string | null>(null);

  const base = apiBase || process.env.NEXT_PUBLIC_API_BASE || "http://localhost:8080/api/v1";

  async function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    if (!e.target.files?.length || busy) return;
    const file = e.target.files[0];
    setBusy(true);
    setError(null);

    const form = new FormData();
    form.append("file", file);

    try {
      const res = await fetch(`${base}/events/${eventId}/image`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
        },
        body: form,
      });

      if (!res.ok) {
        throw new Error(`Upload failed (${res.status})`);
      }

      const body = await res.json();
      const cdnUrl = body.url as string;
      setUrl(cdnUrl);
      onUploaded?.(cdnUrl);
    } catch (err: any) {
      setError(err?.message || "Upload failed");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: "grid", gap: 8, maxWidth: 360 }}>
      <label
        style={{
          display: "inline-block",
          padding: "10px 14px",
          background: busy ? "#6c757d" : "#0d6efd",
          color: "white",
          borderRadius: 8,
          cursor: busy ? "wait" : "pointer",
          textAlign: "center",
        }}
      >
        {busy ? "Uploading..." : "Upload image to CDN"}
        <input
          type="file"
          accept="image/png,image/jpeg,image/webp"
          onChange={handleFileChange}
          style={{ display: "none" }}
          disabled={busy}
        />
      </label>

      {url && (
        <a href={url} target="_blank" rel="noreferrer" style={{ color: "#0d6efd" }}>
          View uploaded image
        </a>
      )}

      {error && <div style={{ color: "crimson" }}>{error}</div>}
    </div>
  );
}
