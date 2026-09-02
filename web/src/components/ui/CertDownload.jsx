import { useState } from 'react';
import { client } from '../../api/client';
import Button from './Button';
import Modal from './Modal';
import CodeBlock from './CodeBlock';
import { useToast } from './Toast';
import { Download, ShieldCheck } from 'lucide-react';

/**
 * Operator-only. This may be imported by /admin/infra and nowhere else — it
 * must never appear in a customer bundle.
 */
export default function CertDownload({ className = '' }) {
  const [showHelp, setShowHelp] = useState(false);
  const [loading, setLoading] = useState(false);
  const toast = useToast();

  async function download() {
    setLoading(true);
    try {
      const res = await client.get('/api/v1/mitm/cert', { responseType: 'blob' });
      const url = URL.createObjectURL(res.data);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'switchblade-ca.pem';
      a.click();
      URL.revokeObjectURL(url);
      toast('Certificate downloaded');
    } catch {
      toast.error('Could not download the certificate. Check that the inspector is running.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <>
      <div className={`flex items-center gap-2 ${className}`}>
        <Button variant="secondary" size="sm" onClick={download} loading={loading}>
          <Download className="w-3.5 h-3.5" aria-hidden="true" />
          CA certificate
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => setShowHelp(true)}
          aria-label="View certificate install instructions"
          title="Install instructions"
        >
          <ShieldCheck className="w-4 h-4" aria-hidden="true" />
        </Button>
      </div>

      <Modal
        open={showHelp}
        onClose={() => setShowHelp(false)}
        title="Install the CA certificate"
        description="Trust the Switchblade CA so intercepted HTTPS traffic validates on this machine."
        footer={
          <div className="flex justify-end">
            <Button variant="secondary" size="sm" onClick={() => setShowHelp(false)}>Close</Button>
          </div>
        }
      >
        <div className="space-y-4">
          <CodeBlock
            tabs={[
              {
                id: 'macos',
                label: 'macOS',
                code: 'sudo security add-trusted-cert -d -r trustRoot \\\n  -k /Library/Keychains/System.keychain switchblade-ca.pem',
              },
              {
                id: 'linux',
                label: 'Linux',
                code: 'sudo cp switchblade-ca.pem /usr/local/share/ca-certificates/switchblade-ca.crt\nsudo update-ca-certificates',
              },
              {
                id: 'windows',
                label: 'Windows',
                code: 'certutil -addstore -f "ROOT" switchblade-ca.pem',
              },
            ]}
          />
          <p className="text-caption text-muted">
            Restart the browser or client after installing so it picks up the new root.
          </p>
        </div>
      </Modal>
    </>
  );
}

export { CertDownload };
