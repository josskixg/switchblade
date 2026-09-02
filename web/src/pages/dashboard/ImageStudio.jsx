import { useEffect, useState } from 'react';
import { client } from '../../api/client';
import { useToast } from '../../components/ui/Toast';
import Card from '../../components/ui/Card';
import Button from '../../components/ui/Button';
import Skeleton from '../../components/ui/Skeleton';
import { Image as ImageIcon, Trash2, HardDrive, ImageOff } from 'lucide-react';

export default function ImageStudio() {
  const [images, setImages] = useState([]);
  const [stats, setStats] = useState(null);
  const [loading, setLoading] = useState(true);
  const toast = useToast();

  useEffect(() => { load(); }, []);

  async function load() {
    try {
      const [imgRes, statsRes] = await Promise.all([
        client.get('/api/images').catch(() => ({ data: [] })),
        client.get('/api/images/stats').catch(() => ({ data: null })),
      ]);
      setImages(imgRes.data || []);
      setStats(statsRes.data);
    } catch {
      toast.error('Failed to load images');
    } finally {
      setLoading(false);
    }
  }

  async function removeImage(id) {
    try {
      await client.delete(`/api/images/${id}`);
      toast('Image deleted');
      load();
    } catch {
      toast.error('Delete failed');
    }
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-text">Image Studio</h1>
        <p className="text-sm text-muted mt-1">Generated images and storage</p>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Card>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <div className="flex items-center gap-4">
              <div className="w-11 h-11 rounded-lg flex items-center justify-center bg-primary/10">
                <ImageIcon className="w-5 h-5 text-primary" />
              </div>
              <div>
                <p className="text-xs text-muted uppercase tracking-wider">Total Images</p>
                <p className="text-2xl font-bold text-text tabular-nums">{stats?.total ?? images.length}</p>
              </div>
            </div>
          )}
        </Card>
        <Card>
          {loading ? (
            <Skeleton className="h-16 w-full" />
          ) : (
            <div className="flex items-center gap-4">
              <div className="w-11 h-11 rounded-lg flex items-center justify-center bg-blue-400/10">
                <HardDrive className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <p className="text-xs text-muted uppercase tracking-wider">Storage Used</p>
                <p className="text-2xl font-bold text-text tabular-nums">
                  {stats?.storage_used ? `${(stats.storage_used / 1024 / 1024).toFixed(1)} MB` : '0 MB'}
                </p>
              </div>
            </div>
          )}
        </Card>
      </div>

      <Card>
        <h3 className="text-base font-semibold text-text mb-4">Gallery</h3>
        {loading ? (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {Array.from({ length: 8 }).map((_, i) => (
              <Skeleton key={i} className="aspect-square w-full rounded-lg" />
            ))}
          </div>
        ) : images.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-12 gap-2 text-muted">
            <ImageOff className="w-8 h-8 text-muted/30" />
            <p className="text-sm">No images generated yet</p>
          </div>
        ) : (
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            {(images || []).map((img) => (
              <div key={img.id} className="group relative rounded-lg overflow-hidden border border-border bg-surface-elevated">
                <img
                  src={img.url}
                  alt={img.prompt || 'Generated image'}
                  className="w-full aspect-square object-cover"
                  loading="lazy"
                  onError={(e) => { e.target.style.display = 'none'; }}
                />
                <div className="p-2">
                  <p className="text-xs text-muted truncate">{img.model || 'unknown'}</p>
                  <p className="text-xs text-muted truncate">{img.prompt || ''}</p>
                </div>
                <button
                  onClick={() => removeImage(img.id)}
                  className="absolute top-2 right-2 p-1.5 rounded-lg bg-danger/80 text-white opacity-0 group-hover:opacity-100 transition-opacity duration-150"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>
    </div>
  );
}
