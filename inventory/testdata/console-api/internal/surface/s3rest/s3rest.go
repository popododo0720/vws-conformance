package s3rest

func New(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.listBuckets)
	mux.HandleFunc("GET /{bucket}", h.listObjects)
	mux.HandleFunc("HEAD /{bucket}", h.headBucket)
	mux.HandleFunc("PUT /{bucket}", h.createBucket)
	mux.HandleFunc("DELETE /{bucket}", h.deleteBucket)
	mux.HandleFunc("PUT /{bucket}/{key...}", h.putObject)
	mux.HandleFunc("GET /{bucket}/{key...}", h.getObject)
	mux.HandleFunc("HEAD /{bucket}/{key...}", h.headObject)
	mux.HandleFunc("DELETE /{bucket}/{key...}", h.deleteObject)
	return mux
}
