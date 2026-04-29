# `redis-afs-sdk`

Python SDK for the AFS control plane and agent filesystem mounts.

```bash
pip install redis-afs-sdk
```

```python
import os
from redis_afs import AFS

afs = AFS(api_key=os.environ["AFS_API_KEY"])
repo = afs.repo.create(name="foobar")
fs = afs.fs.mount(repos=[{"name": repo["name"]}], mode="rw")

fs.write_file("/src/README.md", "hello world")
result = fs.bash().exec("cat /foobar/src/README.md")
print(result.stdout)
fs.close()
```

The API key can also be read automatically from `AFS_API_KEY`:

```bash
export AFS_API_KEY="afs_..."
```

Set `AFS_API_BASE_URL` to point at a local or Self-managed control plane. If not
provided, the SDK defaults to `https://afs.cloud`.

## Test

```bash
PYTHONPATH=src python3 -m unittest discover -s tests
```
