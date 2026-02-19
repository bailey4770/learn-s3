# Learn Serverless Storage with S3 and CDNs with CloudFront

This project is a fork of a basic video uploading website, with the majority of
code coming from [boot.dev](boot.dev).

My additions mostly concern the HTTP handler
funcs, to allow user uploaded videos to be stored in an AWS S3 Bucket and available
worldwide through the CloudFront CDN, using the AWS SDK in Golang. I also worked
in the AWS Console to set up my own S3 buckets and CloudFront CDN. However, these
resources have been deleted to ensure I stay in the AWS free tier, and to avoid
accidental charges.

Below, I discuss some of the key topics covered and lessons learned.

## Browser caching

- Browsers usually cache data retrieved from the internet to save bandwidth and
increase speed on subsequent visits to a page.
- They typically cache based on a URL.
- This becomes a problem if the content on a page changes, but the URL does not.
The browser will serve users stale content.
- Avoid this problem is called *cache busting* and there are several options
available to us:
  1. Add versions to the URL.
  - The browser sees a new URL so fetches fresh content.
  1. A similar method is to generate a new, random URL with each new asset.
  2. The best way is to utilise *cache headers*:
  - It is up to the browser to respect these headers.
  - `no-cache` does not mean don't cache, it meas revalidate content before
      serving.
- My implementation is handled here:

```
func cacheMiddleware(next http.Handler) http.Handler {
 return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
  w.Header().Set("Cache-Control", "max-age=3600")
  next.ServeHTTP(w, r)
 })
}
```

## Large file storage

- Relational databases are perfect for storing text, but not optimised for storing
large files, eg. images or videos.
- Large files are stored as a 'blob' of data, sometimes mega or gigabytes in size.
- We usually serve large static assets from disk.
- In this project, we do this for images, since they are relatively small in size.
  - See `func (cfg *apiConfig) updateThumbnail(...) error` func.

## Single machine vs serverless architecture

- A 'simple' web app uses a single machine to:
  1. Host an HTTP Server to respond to HTTP requests.
  2. Run a database in the background for the server to query.
  3. Store large files in a file system for the server to directly read/write to.
- This is valid, but there are trade-offs:
  - Scaling - as server usage grows, the server's resources must also.
  - Availability - if the server goes down, your app goes down.
  - Durability - you must manage and ensure your own backups/contingency options.
  - Cost - running a server 24/7 means paying 24/7.
- These problems **can** be dealt with, eg. load-balancers, but the key point is:
the server is *your* problem.

- What serverless architecture really means - the server is *someone else's* problem.
- AWS S3 is an example of serverless storage:
  - Rather than serving content from local file storage...
  - Uploaded files are sent to web server, which uploads file to S3 using SDK.
  - Files are then served to clients by passing along the URL to the file in S3's
  servers.
  - See `func (cfg *apiConfig) updateVideo(...) error` for my implementation.
- This archiecture has some key benefits:
  - Solves scaling issue:
    - Scales to zero - if we use zero, we pay zero.
    - Scales to very high - a smaller-medium company is **extremely** unlikely to
    outscale S3's capabilities.
  - Improved availability and redundancy
    - Cloud computing providers have their own specialised teams to ensure both.
    - AWS servers are very rarely unavailabale.
  - Low Cost
    - AWS is **huge**, bringing large economies of scale - lower costs.
- It's worth bearing in mind - you are often *locked-in* when choosing a cloud
computing provider. Although this is not necessrily a bad thing.

## Object/Blob storage

- Serverless storage do not store files in traditional file severs.
- Flat namespaces - there are not 'directories', although AWS does treat file-prefixes
as something similar.
  - This is particularly useful for modifying all files of a particular 'type' or
  from a particular user.
  - Therefore, our storage scheme is important to carefully consider when setting
  up.
- File metadata is stored directly with files.
- This is designed to be more scalable, available, and durable, as it is easier
to distribute files across multiple machines.
  - If there is a fire in one data centre, the other data centres have backups.

## Video streaming

- Streaming allows client to start consuming large files before they are fully downloaded.
- Also allows client to only download relevant data - eg. they've already seen the
beginning of a video, and only want to see the second half.
- This saves time and bandwith - especially important for clients on metered connection,
eg. mobile connection.
- in 2026, streaming is actually quite easy to implement:
  - Native HTML5 `<video>` element streams video by default, so long as the server
  supports it.
  - Range HHTP Headers allows a client to request a specific byte range of data
  from the file.

- MP4 files can easily be processed to enable *fast start*.
- This is implemented in the
`processVideoForFastStart(filePath string) (string, error)`
func.
- We simply use a tool to move the 'moov' atom from the end to the start of the
file.

- There are also other, more specialised streaming approaches.
- Adaptive streaming, such as 'HLS' or 'MPEG-DASH' allow for variable stream quality.
  - Improves user experience on slower connections.
- Low-latency protocols such as 'WebRTC' or 'RTMP' are better suited for live streaming.

## Cloud security

- We need a secure way to connect to our nonlocal storage servers.
- The base level of security is achieved with passwords and secure keys.
- However, these can (and often do) get leaked.
- Principle of least privilege - users should have the minimum permissions necessary
to achieve their tasks.
  - This helps limit the scope of damage that can be done by an attacker using a
  stolen key.
- For added security, we can also add an IP white list to our permissions policy.
  - Attacker would then have to steal keys **and** connect from your local network.
  - However, this can be annoying if devs have dynamic IP addresses.
- Our server code should also have its own identity and permissions.
- We could also run our sever on AWS EC2. Then our code is authenticated entirely
through AWS, and there is no need for us to handle keys, which removes one point
of failure.
- S3 encrypts data at rest - prevents attackers from physically accessing data.
- HHTP**S** layer handles encryption of data in transit, preventing 'man in the
middle' attacks. The **S** being the key here.

## Content Delivery Network (CDN)

- (Typically global) network of servers caching and delivering content to users
based on geographical location.
- If our S3 bucket is hosted in Europe, but a user in Australia wants to access
it, it will take a long time to get that data from Europe.
- CDNs have two main purposes:
  - Speed - users get content from (potentially) closer servers.
  - Security - origin server is hidden from public internet. Only the CDN can
  access it.
- Some CDNs, like CloudFlare (not to be confused with CloudFront!!) are strongly
focussed on security against, eg. DDoS attacks, Web Application Firewalls.

- CDNs have their own difficulties -

> There are only two hard things in Computer Science: cache invalidation and
> naming things.
> -- Phil Karlton

- A cache invalidation is when the cached content becomes stale - not up to date.
- In AWS, we can trigger invalidations through the console (as I did), or using
the SDK.
- In prod, we would likely want to trigger one programmatically when a user updates
a critical piece of information, eg. posts a new video.
- The difficulties of cache invalidation is yet another reason smaller businesses
can benefit from relying on cloud computing to handle to difficult parts.
